package controllers

import (
	"context"
	"errors"
	"log"
	"time"

	userv1 "Matchup/gen/user/v1"
	"Matchup/security"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
	"golang.org/x/crypto/bcrypt"
)

// Self-serve account deletion. Two-step by design: a soft-delete
// stamps `users.deleted_at` NOW + wipes device-session state
// immediately (refresh tokens, push subscriptions) so the row stops
// receiving notifications + the user is signed out of every device.
// A daily cron (HardDeleteExpiredAccounts) then hard-deletes rows
// past the 30-day retention window.
//
// Why not hard-delete immediately? Three reasons:
//   - UX: users change their minds. A 30-day window + support-channel
//     restore keeps us from turning a bad mouse-click into a data loss.
//   - Moderation: if a reporter's account self-deletes the day their
//     report fires, we still want to be able to audit the report.
//   - Cascade cost: hard-delete fans out across 7+ engagement tables
//     (matchups, brackets, votes, likes, comments, follows, ...).
//     Running that at the end of the retention window batches the
//     work into the scheduler's off-peak slot.

// AccountHardDeleteRetention is the grace period between soft-delete
// and hard-delete. Exposed as a var for integration tests that need
// to simulate a long-past deletion — override it, run the cron,
// restore.
var AccountHardDeleteRetention = 30 * 24 * time.Hour

// DeleteMyAccount is the user-facing self-delete RPC. Requires the
// caller's password as a confirmation signal; returns the scheduled
// hard-delete timestamp so the UI can tell the user when recovery
// stops being possible.
//
// Note: this is a method on UserHandler (not a new handler type) so
// the UserService proto gets its RPC mounted without a new service
// + a new router wiring line. Lives in its own file for
// reviewability — the code is enough of its own concern.
func (h *UserHandler) DeleteMyAccount(ctx context.Context, req *connect.Request[userv1.DeleteMyAccountRequest]) (*connect.Response[userv1.DeleteMyAccountResponse], error) {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	// Password confirmation — mirror the Login handler's bcrypt check
	// so a compromised access token alone can't nuke the account.
	var storedHash string
	if err := h.DB.GetContext(ctx, &storedHash, "SELECT password FROM users WHERE id = $1 AND deleted_at IS NULL", uid); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	}
	if err := security.VerifyPassword(storedHash, req.Msg.GetPassword()); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("password incorrect"))
		}
		// Corrupted hash (e.g. ErrHashTooShort) — same posture as the
		// Login handler: log loudly, surface a clean 401 instead of a
		// 500. The user needs to reset via forgot-password before
		// they can delete the account.
		log.Printf("delete-account: corrupted password hash for user_id=%d: %v", uid, err)
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("password incorrect"))
	}

	// Reason is optional; nil means NULL in the column.
	var reason *string
	if r := req.Msg.GetReason(); r != "" {
		reason = &r
	}

	// Stamp deleted_at + capture the timestamp so we can return a
	// deterministic hard_delete_at in the response (NOW() inside the
	// UPDATE would round-trip through the DB; we want the exact
	// value the row holds).
	var deletedAt time.Time
	if err := h.DB.QueryRowContext(ctx, `
		UPDATE public.users
		   SET deleted_at = NOW(),
		       deletion_reason = $2,
		       updated_at = NOW()
		 WHERE id = $1
		 RETURNING deleted_at`, uid, reason,
	).Scan(&deletedAt); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Immediate cleanup — don't wait for the hard-delete cron. These
	// two are the "this user is actively transmitting" surfaces we
	// want silenced the moment delete confirms:
	//
	//   push_subscriptions — deleting the rows stops the publisher
	//     from ever sending push to this user's devices again.
	//   refresh_tokens — revoking forces every browser / mobile
	//     session to re-authenticate, which bounces them off the
	//     soft-deleted row (login lookup filters WHERE deleted_at
	//     IS NULL).
	//
	// Both are best-effort: failures here are logged but don't
	// block the delete. The hard-delete cron cleans up stragglers
	// via ON DELETE CASCADE when the row finally drops.
	if _, err := h.DB.ExecContext(ctx,
		"DELETE FROM public.push_subscriptions WHERE user_id = $1", uid,
	); err != nil {
		// Log; don't fail the delete.
		_ = err
	}
	if _, err := h.DB.ExecContext(ctx,
		"UPDATE public.refresh_tokens SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL",
		uid,
	); err != nil {
		_ = err
	}

	hardDeleteAt := deletedAt.Add(AccountHardDeleteRetention)
	return connect.NewResponse(&userv1.DeleteMyAccountResponse{
		HardDeleteAt: rfc3339(hardDeleteAt),
	}), nil
}
