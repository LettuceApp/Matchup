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
	if strings.Contains(identifier, "@") {
		lookupErr = h.DB.GetContext(ctx, &user, "SELECT * FROM users WHERE lower(email) = $1", identifier)
	} else {
		lookupErr = h.DB.GetContext(ctx, &user, "SELECT * FROM users WHERE lower(username) = $1", identifier)
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

	user.AvatarPath = appdb.ProcessAvatarPath(user.AvatarPath)

	// Merge anonymous device votes if the client supplies X-Anon-Id header.
	if anonID := req.Header().Get("X-Anon-Id"); anonID != "" {
		if err := mergeDeviceVotesToUser(h.DB, user.ID, anonID); err != nil {
			log.Printf("merge anonymous votes on login: %v", err)
		}
	}

	return connect.NewResponse(&authv1.LoginResponse{
		Token:      token,
		Id:         user.PublicID,
		Email:      user.Email,
		Username:   user.Username,
		IsAdmin:    user.IsAdmin,
		AvatarPath: user.AvatarPath,
		IsPrivate:  user.IsPrivate,
	}), nil
}

func (h *AuthHandler) ForgotPassword(ctx context.Context, req *connect.Request[authv1.ForgotPasswordRequest]) (*connect.Response[authv1.ForgotPasswordResponse], error) {
	email := strings.ToLower(strings.TrimSpace(req.Msg.Email))
	if email == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("email is required"))
	}

	var user models.User
	if err := h.DB.GetContext(ctx, &user, "SELECT * FROM users WHERE email = $1", email); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("we do not recognize this email"))
	}

	var rp models.ResetPassword
	rp.Prepare()
	rp.Email = email
	rp.Token = security.TokenHash(email)

	resetDetails, err := rp.SaveDatails(h.DB)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Static success message — we never wait for Sendgrid before
	// responding. The user sees a fast 200 regardless of email
	// provider latency.
	const successMessage = "Success, Please click on the link provided in your email"

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
			return connect.NewResponse(&authv1.ForgotPasswordResponse{Message: successMessage}), nil
		} else {
			log.Printf("forgot password: enqueue failed, falling back to inline send: %v", err)
		}
	}

	// Inline fallback — keeps dev environments without Redis working.
	response, err := mailer.SendMail.SendResetPassword(
		resetDetails.Email,
		os.Getenv("SENDGRID_FROM"),
		resetDetails.Token,
		os.Getenv("SENDGRID_API_KEY"),
		os.Getenv("APP_ENV"),
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&authv1.ForgotPasswordResponse{
		Message: response.RespBody,
	}), nil
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

	var rp models.ResetPassword
	if err := h.DB.GetContext(ctx, &rp, "SELECT * FROM reset_passwords WHERE token = $1", req.Msg.Token); err != nil {
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
