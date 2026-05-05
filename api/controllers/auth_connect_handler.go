package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"Matchup/auth"
	"Matchup/cache"
	appdb "Matchup/db"
	authv1 "Matchup/gen/auth/v1"
	"Matchup/gen/auth/v1/authv1connect"
	"Matchup/jobs"
	"Matchup/jobs/handlers"
	"Matchup/mailer"
	"Matchup/models"
	"Matchup/security"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler implements authv1connect.AuthServiceHandler.
type AuthHandler struct{ DB *sqlx.DB }

var _ authv1connect.AuthServiceHandler = (*AuthHandler)(nil)

func (h *AuthHandler) Login(ctx context.Context, req *connect.Request[authv1.LoginRequest]) (*connect.Response[authv1.LoginResponse], error) {
	var user models.User
	identifier := strings.ToLower(strings.TrimSpace(req.Msg.Email))
	var lookupErr error
	// `deleted_at IS NULL` blocks self-deleted + admin-banned accounts
	// from signing in. Returns the same "incorrect email or password"
	// error as a non-existent email so we don't leak which accounts
	// were banned vs never-existed.
	if strings.Contains(identifier, "@") {
		lookupErr = h.DB.GetContext(ctx, &user, "SELECT * FROM users WHERE lower(email) = $1 AND deleted_at IS NULL", identifier)
	} else {
		lookupErr = h.DB.GetContext(ctx, &user, "SELECT * FROM users WHERE lower(username) = $1 AND deleted_at IS NULL", identifier)
	}
	if lookupErr != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("incorrect email or password"))
	}
	if err := security.VerifyPassword(user.Password, req.Msg.Password); err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("incorrect email or password"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	token, err := auth.CreateToken(user.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Mint a refresh token to pair with the access token. user_agent
	// is optional debugging metadata — the connect request may not
	// carry it in all transports.
	refreshPlaintext, err := auth.MintRefreshToken(ctx, h.DB, user.ID, req.Header().Get("User-Agent"))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	user.AvatarPath = appdb.ProcessAvatarPath(user.AvatarPath)

	// Merge anonymous device votes if the client supplies X-Anon-Id header.
	if anonID := req.Header().Get("X-Anon-Id"); anonID != "" {
		if err := mergeDeviceVotesToUser(h.DB, user.ID, anonID); err != nil {
			log.Printf("merge anonymous votes on login: %v", err)
		}
	}

	return connect.NewResponse(&authv1.LoginResponse{
		Token:                 token,
		Id:                    user.PublicID,
		Email:                 user.Email,
		Username:              user.Username,
		IsAdmin:               user.IsAdmin,
		AvatarPath:            user.AvatarPath,
		IsPrivate:             user.IsPrivate,
		RefreshToken:          refreshPlaintext,
		AccessTokenExpiresIn:  int64(auth.AccessTokenTTL.Seconds()),
	}), nil
}

// Refresh rotates the caller's refresh token into a new one and mints
// a fresh access token. This is the only RPC on the Auth service that
// accepts a refresh token as the credential; the access-token-protected
// routes use the shorter-lived JWT.
//
// All sentinel errors from auth.RotateRefreshToken map to
// Unauthenticated with the same opaque message — attackers must not
// be able to probe which specific state a token is in.
func (h *AuthHandler) Refresh(ctx context.Context, req *connect.Request[authv1.RefreshRequest]) (*connect.Response[authv1.RefreshResponse], error) {
	plaintext := req.Msg.GetRefreshToken()
	if plaintext == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("refresh_token is required"))
	}

	newPlaintext, userID, err := auth.RotateRefreshToken(ctx, h.DB, plaintext, req.Header().Get("User-Agent"))
	if err != nil {
		// Log the specific sentinel so ops can tell routine expiry
		// apart from theft signals; return a generic error to the caller.
		log.Printf("refresh: %v", err)
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid refresh token"))
	}

	accessToken, err := auth.CreateToken(userID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&authv1.RefreshResponse{
		Token:                accessToken,
		RefreshToken:         newPlaintext,
		AccessTokenExpiresIn: int64(auth.AccessTokenTTL.Seconds()),
	}), nil
}

// Logout revokes the refresh token the caller presents. Auth-required
// (we want to know which user is revoking, for audit + rate-limit),
// but the server does NOT cross-check that the refresh token belongs
// to that user — the token_hash lookup alone is sufficient, and a
// user revoking someone else's stolen token is a feature, not a bug.
// Always 200-OK: telling a caller "that token doesn't exist" would
// leak liveness information.
func (h *AuthHandler) Logout(ctx context.Context, req *connect.Request[authv1.LogoutRequest]) (*connect.Response[authv1.LogoutResponse], error) {
	if _, ok := httpctx.CurrentUserID(ctx); !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}
	if err := auth.RevokeRefreshToken(ctx, h.DB, req.Msg.GetRefreshToken()); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&authv1.LogoutResponse{}), nil
}

// LogoutAll revokes every non-revoked refresh token the caller owns.
// Scoped strictly by the auth-middleware-provided uid.
func (h *AuthHandler) LogoutAll(ctx context.Context, _ *connect.Request[authv1.LogoutAllRequest]) (*connect.Response[authv1.LogoutAllResponse], error) {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}
	n, err := auth.RevokeAllUserTokens(ctx, h.DB, uid)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&authv1.LogoutAllResponse{RevokedCount: int32(n)}), nil
}

// ForgotPassword returns a fixed neutral response whether or not the
// email exists in the database. Revealing non-existence would let an
// attacker enumerate valid accounts one POST at a time. The email
// itself (if we actually have one for this address) is fired off
// asynchronously via the email worker; the handler returns without
// waiting for SendGrid.
func (h *AuthHandler) ForgotPassword(ctx context.Context, req *connect.Request[authv1.ForgotPasswordRequest]) (*connect.Response[authv1.ForgotPasswordResponse], error) {
	email := strings.ToLower(strings.TrimSpace(req.Msg.Email))
	if email == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("email is required"))
	}

	// Neutral, identical response for both known + unknown emails —
	// the phrasing below deliberately doesn't promise an email was
	// sent, only that one will be sent "if the account exists".
	const neutralMessage = "If that email is registered, a reset link has been sent."

	var user models.User
	if err := h.DB.GetContext(ctx, &user, "SELECT * FROM users WHERE email = $1 AND deleted_at IS NULL", email); err != nil {
		// Unknown email → return neutral 200 without touching the
		// reset_passwords table. An observer who times requests can
		// still distinguish hit vs miss; that's acceptable for now
		// (a follow-up cycle can add a dummy bcrypt hash delay).
		return connect.NewResponse(&authv1.ForgotPasswordResponse{Message: neutralMessage}), nil
	}

	var rp models.ResetPassword
	rp.Prepare()
	rp.Email = email
	rp.Token = security.TokenHash(email)

	resetDetails, err := rp.SaveDatails(h.DB)
	if err != nil {
		// Still respond neutrally — don't leak DB state.
		log.Printf("forgot password: save token failed: %v", err)
		return connect.NewResponse(&authv1.ForgotPasswordResponse{Message: neutralMessage}), nil
	}

	// Hand the actual SendGrid call off to the worker. If Redis is
	// unavailable (e.g. local dev without `cache` configured), fall
	// back to the synchronous path so password resets still work.
	payload, marshalErr := json.Marshal(handlers.EmailJob{
		Kind:  handlers.EmailKindResetPassword,
		To:    resetDetails.Email,
		Token: resetDetails.Token,
	})
	if marshalErr == nil && cache.Client != nil {
		if err := jobs.Enqueue(ctx, handlers.EmailQueue, payload); err == nil {
			return connect.NewResponse(&authv1.ForgotPasswordResponse{Message: neutralMessage}), nil
		} else {
			log.Printf("forgot password: enqueue failed, falling back to inline send: %v", err)
		}
	}

	// Inline fallback — keeps dev environments without Redis working.
	if _, err := mailer.SendMail.SendResetPassword(
		resetDetails.Email,
		os.Getenv("SENDGRID_FROM"),
		resetDetails.Token,
		os.Getenv("SENDGRID_API_KEY"),
		os.Getenv("APP_ENV"),
	); err != nil {
		log.Printf("forgot password: inline send failed: %v", err)
	}

	return connect.NewResponse(&authv1.ForgotPasswordResponse{Message: neutralMessage}), nil
}

func (h *AuthHandler) ResetPassword(ctx context.Context, req *connect.Request[authv1.ResetPasswordRequest]) (*connect.Response[authv1.ResetPasswordResponse], error) {
	if req.Msg.Token == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("token is required"))
	}
	if req.Msg.NewPassword == "" || req.Msg.RetypePassword == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("both password fields are required"))
	}
	if len(req.Msg.NewPassword) < 6 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("password must be at least 6 characters"))
	}
	if req.Msg.NewPassword != req.Msg.RetypePassword {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("passwords do not match"))
	}

	// 2-hour TTL for reset links — matches industry norm (GitHub, Google,
	// Discord all land around 1–2 hours). The `AND created_at > ...`
	// filter collapses the "missing row" and "expired row" cases into
	// the same InvalidArgument response so an attacker can't
	// distinguish between them. A nightly cron hard-deletes rows older
	// than 24 h to keep the table tiny (see
	// scheduler.go:jobDeleteExpiredResetTokens).
	var rp models.ResetPassword
	if err := h.DB.GetContext(ctx, &rp,
		`SELECT * FROM reset_passwords
		  WHERE token = $1
		    AND created_at > NOW() - INTERVAL '2 hours'`,
		req.Msg.Token,
	); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid or expired token"))
	}

	user := models.User{Email: rp.Email, Password: req.Msg.NewPassword}
	user.Prepare()
	if err := user.UpdatePassword(h.DB); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if _, err := rp.DeleteDetails(h.DB); err != nil {
		log.Printf("failed to delete reset token: %v", err)
	}

	return connect.NewResponse(&authv1.ResetPasswordResponse{
		Message: "Password reset successfully",
	}), nil
}

// DeleteExpiredResetTokens drops reset-password rows older than 24 h.
// Called from the scheduler nightly. The 2-hour TTL already prevents
// abuse; this keeps the table from growing unbounded as every
// forgot-password request adds a row whether it's used or not.
func DeleteExpiredResetTokens(ctx context.Context, db *sqlx.DB) error {
	res, err := db.ExecContext(ctx,
		`DELETE FROM reset_passwords WHERE created_at < NOW() - INTERVAL '24 hours'`,
	)
	if err != nil {
		return fmt.Errorf("DeleteExpiredResetTokens: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("reset-token cleanup: removed %d expired rows", n)
	}
	return nil
}
