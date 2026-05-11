package controllers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	authv1 "Matchup/gen/auth/v1"
	"Matchup/cache"
	"Matchup/jobs"
	"Matchup/jobs/handlers"
	"Matchup/mailer"
	"Matchup/middlewares"
	"Matchup/models"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
)

// Email verification (soft-nudge mode) — handlers hang off the
// AuthHandler (the proto declares them on AuthService). Kept in a
// separate file so the auth connect handler stays legible.
//
// Token flow:
//   1. Server mints a random 32-byte base64url token (shown to user).
//   2. At rest we store only SHA-256(token) in email_verification_tokens.
//   3. User clicks the link → ConfirmEmailVerification hashes the
//      incoming token, looks up the row, checks expiry, stamps
//      users.email_verified_at + tokens.used_at in one transaction.
//
// Reusing the refresh-token hash-at-rest pattern means a DB dump can't
// be weaponised to mint valid confirmation links for someone else's
// email.

// EmailVerificationTokenTTL — how long a minted link stays valid.
// 24 hours matches Apple/Google/Discord norms. Overridable in tests.
var EmailVerificationTokenTTL = 24 * time.Hour

// requestEmailVerificationBucket + budget for the per-user rate limit.
// 5/hour lets a real user click "Resend" a few times if their inbox
// is slow, but stops "resend forever" from being a spam vector.
const (
	requestEmailVerificationBucket = "email_verify_request"
	requestEmailVerificationBudget = 5
)

// RequestEmailVerification mints a fresh token for the caller and
// enqueues the verification email. Auth-required — you can only
// verify your own email. Deliberately neutral response whether the
// enqueue succeeded or not; the client can retry.
func (h *AuthHandler) RequestEmailVerification(ctx context.Context, req *connect.Request[authv1.RequestEmailVerificationRequest]) (*connect.Response[authv1.RequestEmailVerificationResponse], error) {
	userID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}

	allowed, err := middlewares.CheckUserRateLimit(ctx, userID, requestEmailVerificationBucket, requestEmailVerificationBudget, time.Hour)
	if err != nil {
		// Rate-limit store errors are non-fatal — log + proceed so a
		// Redis outage doesn't lock a real user out of resending.
		log.Printf("verify email rate-limit check: %v", err)
	} else if !allowed {
		return nil, connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("too many verification requests; try again later"))
	}

	var user models.User
	if err := h.DB.GetContext(ctx, &user,
		"SELECT * FROM users WHERE id = $1 AND deleted_at IS NULL", userID,
	); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}
	if user.EmailVerifiedAt != nil {
		// Idempotent — already verified, no-op with a friendly
		// message. Lets the frontend call this defensively on
		// component mount without branching.
		return connect.NewResponse(&authv1.RequestEmailVerificationResponse{
			Message: "Email already verified.",
		}), nil
	}

	token, err := mintEmailVerificationToken(ctx, h.DB, user.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("mint token: %w", err))
	}

	dispatchVerifyEmail(ctx, user.Email, token)
	return connect.NewResponse(&authv1.RequestEmailVerificationResponse{
		Message: "Verification email sent.",
	}), nil
}

// dispatchVerifyEmail is the single source of truth for "send a
// verification email to address X with token T". Used by both
// RequestEmailVerification (the resend RPC) and the CreateUser
// auto-mint path so the dispatch logic — and its quirks — only live
// in one place.
//
// Behaviour:
//   * In production with Redis available, hand off to the email
//     worker via the queue. Returns immediately so the calling
//     handler doesn't block on SendGrid latency.
//   * In any other case (dev, missing Redis, missing worker
//     process), do an inline SendGrid call. The inline path also
//     fires when the queue enqueue itself errors — best-effort.
//   * Errors are logged but never bubble up. Verification is a
//     fire-and-forget side effect; the user can always click
//     "Resend" if the email didn't arrive.
//
// The dev preference for inline is deliberate: a typical local
// stack runs only the API binary, NOT cmd/worker. Routing to the
// queue in dev would leave the message un-drained and the email
// never sent — exactly the bug this helper exists to prevent.
func dispatchVerifyEmail(ctx context.Context, toEmail, token string) {
	if shouldUseEmailQueue() {
		payload, marshalErr := json.Marshal(handlers.EmailJob{
			Kind:  handlers.EmailKindVerifyEmail,
			To:    toEmail,
			Token: token,
		})
		if marshalErr == nil {
			if err := jobs.Enqueue(ctx, handlers.EmailQueue, payload); err == nil {
				return
			} else {
				log.Printf("verify email: enqueue failed, falling back to inline send: %v", err)
			}
		} else {
			log.Printf("verify email: marshal payload failed, falling back to inline send: %v", marshalErr)
		}
	}

	if _, err := mailer.SendMail.SendVerifyEmail(
		toEmail,
		os.Getenv("SENDGRID_FROM"),
		token,
		os.Getenv("SENDGRID_API_KEY"),
		os.Getenv("APP_ENV"),
	); err != nil {
		log.Printf("verify email: inline send failed: %v", err)
	}
}

// shouldUseEmailQueue returns true when the queue path is preferred
// (production deploy with a worker draining the email queue). Local
// dev gets the inline path so signup verification works without
// running a separate cmd/worker process.
//
// EMAIL_QUEUE_DISABLED is an explicit opt-out for single-service
// deployments where Redis is connected (so cache.Client != nil) but
// no cmd/worker is running to drain the queue — Render and Fly
// without a separate worker process are the common cases. Set this
// to "true" in those environments to force the inline-send fallback,
// otherwise enqueued emails sit in Redis forever and the user wonders
// why no email arrived.
func shouldUseEmailQueue() bool {
	if strings.EqualFold(os.Getenv("EMAIL_QUEUE_DISABLED"), "true") {
		return false
	}
	if cache.Client == nil {
		return false
	}
	return os.Getenv("APP_ENV") == "production"
}

// ConfirmEmailVerification is anonymous — the user might click the
// link from a different device than the one they signed up on.
// Token goes through SHA-256 lookup + expiry check + single-use
// transaction.
func (h *AuthHandler) ConfirmEmailVerification(ctx context.Context, req *connect.Request[authv1.ConfirmEmailVerificationRequest]) (*connect.Response[authv1.ConfirmEmailVerificationResponse], error) {
	rawToken := req.Msg.GetToken()
	if rawToken == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("token is required"))
	}
	hashed := hashEmailVerificationToken(rawToken)

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer func() { _ = tx.Rollback() }()

	// Lock the row so two concurrent clicks don't both succeed.
	var row struct {
		ID        int64        `db:"id"`
		UserID    uint         `db:"user_id"`
		ExpiresAt time.Time    `db:"expires_at"`
		UsedAt    sql.NullTime `db:"used_at"`
	}
	if err := tx.GetContext(ctx, &row,
		`SELECT id, user_id, expires_at, used_at
		   FROM public.email_verification_tokens
		  WHERE token_hash = $1
		  FOR UPDATE`,
		hashed,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid or expired token"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if row.UsedAt.Valid {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("token already used"))
	}
	if time.Now().After(row.ExpiresAt) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid or expired token"))
	}

	// Stamp both sides atomically. If the user row has already been
	// stamped by a concurrent request, COALESCE keeps the earlier
	// verification timestamp (idempotent).
	if _, err := tx.ExecContext(ctx,
		`UPDATE public.users
		    SET email_verified_at = COALESCE(email_verified_at, NOW())
		  WHERE id = $1`,
		row.UserID,
	); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE public.email_verification_tokens SET used_at = NOW() WHERE id = $1`,
		row.ID,
	); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&authv1.ConfirmEmailVerificationResponse{
		Message: "Email verified.",
	}), nil
}

// ---- helpers ----

// mintEmailVerificationToken creates a fresh token for the given user.
// Returns the raw token (to send in the email) — the DB only stores
// the hash. Expires EmailVerificationTokenTTL from now.
//
// Called from Register + RequestEmailVerification. Keeps both paths
// consistent without each reimplementing the random-bytes shape.
func mintEmailVerificationToken(ctx context.Context, db sqlx.ExtContext, userID uint) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read rand: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(buf)
	hashed := hashEmailVerificationToken(raw)
	expires := time.Now().Add(EmailVerificationTokenTTL)

	if _, err := db.ExecContext(ctx,
		`INSERT INTO public.email_verification_tokens (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		userID, hashed, expires,
	); err != nil {
		return "", fmt.Errorf("insert token: %w", err)
	}
	return raw, nil
}

// hashEmailVerificationToken returns the SHA-256 hex digest of the raw
// token — what we store + compare. Hex (not base64) so the column is
// constant-width, which makes the UNIQUE index predictable.
func hashEmailVerificationToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// errUnverifiedContentCreate is the canonical Connect error that write
// handlers return when an unverified user tries to create content.
// FailedPrecondition (not PermissionDenied) signals "the caller is
// authed, but an account state flag needs flipping before this action
// is allowed" — the frontend maps that specific code to the "Verify
// your email" modal rather than a generic 403 toast.
var errUnverifiedContentCreate = connect.NewError(
	connect.CodeFailedPrecondition,
	fmt.Errorf("verify your email to create content"),
)

// requireVerifiedEmail checks the viewer's email_verified_at flag and
// returns errUnverifiedContentCreate if NULL. Called from the top of
// CreateMatchup, CreateBracket, and CreateComment (when the author
// isn't commenting on their own matchup). ctx must carry a user — the
// handler auth check has already confirmed that upstream.
//
// Deliberately narrow scope: only the three write surfaces call this.
// Voting, liking, following, and profile edits stay open so review
// bots can exercise the app end-to-end without tripping the gate.
func requireVerifiedEmail(ctx context.Context, db sqlx.ExtContext, userID uint) error {
	// Self-disable when no mailer is configured. Without SENDGRID_API_KEY
	// set, RequestEmailVerification can mint a token but
	// dispatchVerifyEmail's inline send will fail silently — the user
	// has no path to actually verify. Gating in that state would trap
	// every signup permanently. Cheap belt-and-suspenders against an
	// outage where SendGrid (or its config) goes away mid-deploy.
	if os.Getenv("SENDGRID_API_KEY") == "" {
		return nil
	}
	if userID == 0 {
		// Not logged in — the handler's own auth check returns
		// Unauthenticated first, so we should never hit this branch
		// in practice. Guard defensively to avoid a panic if the
		// ordering ever changes.
		return nil
	}
	var verifiedAt sql.NullTime
	if err := sqlx.GetContext(ctx, db, &verifiedAt,
		"SELECT email_verified_at FROM users WHERE id = $1", userID,
	); err != nil {
		// On lookup failure, fail-open — otherwise a transient DB
		// hiccup would block all gated actions for everyone.
		log.Printf("requireVerifiedEmail lookup: %v", err)
		return nil
	}
	if !verifiedAt.Valid {
		return errUnverifiedContentCreate
	}
	return nil
}
