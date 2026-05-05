//go:build integration

package controllers

import (
	"context"
	"testing"
	"time"

	authv1 "Matchup/gen/auth/v1"

	"connectrpc.com/connect"
)

// TestForgotPassword_UnknownEmailNeutralResponse — the handler must
// return the same neutral 200 whether the email exists or not, so an
// attacker can't enumerate accounts.
func TestForgotPassword_UnknownEmailNeutralResponse(t *testing.T) {
	db := setupTestDB(t)

	h := &AuthHandler{DB: db}
	resp, err := h.ForgotPassword(context.Background(), connectReq(&authv1.ForgotPasswordRequest{
		Email: "nobody@example.com",
	}, ""))
	if err != nil {
		t.Fatalf("ForgotPassword: %v", err)
	}
	if resp.Msg.Message == "" {
		t.Error("expected a neutral message, got empty string")
	}

	// No reset row should have been created for a ghost email — the
	// handler short-circuits before the INSERT when the user lookup
	// fails.
	var count int
	_ = db.Get(&count, "SELECT COUNT(*) FROM reset_passwords WHERE email = $1", "nobody@example.com")
	if count != 0 {
		t.Errorf("expected 0 reset rows for unknown email, got %d", count)
	}
}

// TestForgotPassword_KnownEmailCreatesToken — happy path. A real
// user's email gets a row in reset_passwords that can then be used
// to reset the password.
func TestForgotPassword_KnownEmailCreatesToken(t *testing.T) {
	db := setupTestDB(t)
	u := seedTestUser(t, db, "alice", "alice@example.com", "TestPass123")

	h := &AuthHandler{DB: db}
	if _, err := h.ForgotPassword(context.Background(), connectReq(&authv1.ForgotPasswordRequest{
		Email: u.Email,
	}, "")); err != nil {
		t.Fatalf("ForgotPassword: %v", err)
	}

	var count int
	_ = db.Get(&count, "SELECT COUNT(*) FROM reset_passwords WHERE email = $1", u.Email)
	if count != 1 {
		t.Errorf("expected 1 reset row, got %d", count)
	}
}

// TestResetPassword_SuccessAndOneTimeUse — first call succeeds;
// second call with the same token errors because the row is deleted
// after use.
func TestResetPassword_SuccessAndOneTimeUse(t *testing.T) {
	db := setupTestDB(t)
	u := seedTestUser(t, db, "alice", "alice@example.com", "TestPass123")

	// Seed a reset row directly — the neutral response on
	// ForgotPassword makes it impractical to read the token out of
	// the RPC response.
	token := "fresh-token-abcdefg"
	if _, err := db.Exec(
		`INSERT INTO reset_passwords (email, token, created_at, updated_at)
		 VALUES ($1, $2, NOW(), NOW())`,
		u.Email, token,
	); err != nil {
		t.Fatalf("seed reset row: %v", err)
	}

	h := &AuthHandler{DB: db}
	if _, err := h.ResetPassword(context.Background(), connectReq(&authv1.ResetPasswordRequest{
		Token:          token,
		NewPassword:    "NewPass456",
		RetypePassword: "NewPass456",
	}, "")); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}

	// Row should be gone (one-time use).
	var remaining int
	_ = db.Get(&remaining, "SELECT COUNT(*) FROM reset_passwords WHERE token = $1", token)
	if remaining != 0 {
		t.Errorf("expected reset row to be deleted after use, got %d", remaining)
	}

	// Second call with the same token → InvalidArgument.
	_, err := h.ResetPassword(context.Background(), connectReq(&authv1.ResetPasswordRequest{
		Token:          token,
		NewPassword:    "Whatever789",
		RetypePassword: "Whatever789",
	}, ""))
	assertConnectError(t, err, connect.CodeInvalidArgument)
}

// TestResetPassword_TTLExpiry — a row older than 2 hours is rejected
// even though it still exists in the DB. Pre-dates created_at to
// simulate an old token the cleanup cron hasn't removed yet.
func TestResetPassword_TTLExpiry(t *testing.T) {
	db := setupTestDB(t)
	u := seedTestUser(t, db, "alice", "alice@example.com", "TestPass123")

	token := "aged-token-123"
	if _, err := db.Exec(
		`INSERT INTO reset_passwords (email, token, created_at, updated_at)
		 VALUES ($1, $2, $3, $3)`,
		u.Email, token, time.Now().Add(-3*time.Hour),
	); err != nil {
		t.Fatalf("seed aged row: %v", err)
	}

	h := &AuthHandler{DB: db}
	_, err := h.ResetPassword(context.Background(), connectReq(&authv1.ResetPasswordRequest{
		Token:          token,
		NewPassword:    "NewPass456",
		RetypePassword: "NewPass456",
	}, ""))
	assertConnectError(t, err, connect.CodeInvalidArgument)

	// Row still exists — TTL check doesn't delete, the cron handles
	// cleanup. This assertion pins that separation of concerns.
	var count int
	_ = db.Get(&count, "SELECT COUNT(*) FROM reset_passwords WHERE token = $1", token)
	if count != 1 {
		t.Errorf("expected expired row to remain in place (handler doesn't delete), got %d", count)
	}
}

// TestDeleteExpiredResetTokens_SweepsOldRows — the cron helper drops
// rows older than 24 hours and leaves newer rows alone.
func TestDeleteExpiredResetTokens_SweepsOldRows(t *testing.T) {
	db := setupTestDB(t)
	u := seedTestUser(t, db, "alice", "alice@example.com", "TestPass123")

	for _, age := range []time.Duration{25 * time.Hour, 48 * time.Hour, 1 * time.Hour} {
		if _, err := db.Exec(
			`INSERT INTO reset_passwords (email, token, created_at, updated_at)
			 VALUES ($1, gen_random_uuid()::text, $2, $2)`,
			u.Email, time.Now().Add(-age),
		); err != nil {
			t.Fatalf("seed aged row (%v): %v", age, err)
		}
	}

	if err := DeleteExpiredResetTokens(context.Background(), db); err != nil {
		t.Fatalf("DeleteExpiredResetTokens: %v", err)
	}

	var remaining int
	_ = db.Get(&remaining, "SELECT COUNT(*) FROM reset_passwords")
	if remaining != 1 {
		t.Errorf("expected 1 young row to survive, got %d", remaining)
	}
}
