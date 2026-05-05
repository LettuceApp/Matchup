//go:build integration

package controllers

import (
	"context"
	"testing"
	"time"

	authv1 "Matchup/gen/auth/v1"
	userv1 "Matchup/gen/user/v1"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
)

// TestDeleteMyAccount_StampsDeletedAtAndWipesSessions — the happy
// path: correct password → deleted_at set, deletion_reason stored,
// push subscriptions deleted, refresh tokens revoked.
func TestDeleteMyAccount_StampsDeletedAtAndWipesSessions(t *testing.T) {
	db := setupTestDB(t)
	t.Setenv("API_SECRET", "integration-test-secret")

	u := seedTestUser(t, db, "goodbye", "bye@example.com", "TestPass123")

	// Seed a push subscription + a refresh token so we can verify
	// the inline cleanup fires.
	if _, err := db.Exec(
		`INSERT INTO push_subscriptions (user_id, platform, endpoint, p256dh_key, auth_key)
		 VALUES ($1, 'web', 'https://fcm.example/abc', 'p', 'a')`,
		u.ID,
	); err != nil {
		t.Fatalf("seed push: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO refresh_tokens (user_id, token_hash, family_id, expires_at)
		 VALUES ($1, 'deadbeef', gen_random_uuid(), NOW() + INTERVAL '30 days')`,
		u.ID,
	); err != nil {
		t.Fatalf("seed refresh: %v", err)
	}

	handler := &UserHandler{DB: db}
	ctx := httpctx.WithUserID(context.Background(), u.ID)

	reason := "not using it anymore"
	resp, err := handler.DeleteMyAccount(ctx, connectReq(&userv1.DeleteMyAccountRequest{
		Password: "TestPass123",
		Reason:   &reason,
	}, ""))
	if err != nil {
		t.Fatalf("DeleteMyAccount: %v", err)
	}
	if resp.Msg.HardDeleteAt == "" {
		t.Error("expected hard_delete_at to be set")
	}

	// users.deleted_at now stamped + reason stored.
	var deletedAt *time.Time
	var storedReason *string
	if err := db.QueryRow(
		"SELECT deleted_at, deletion_reason FROM users WHERE id = $1", u.ID,
	).Scan(&deletedAt, &storedReason); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if deletedAt == nil {
		t.Error("deleted_at should be stamped")
	}
	if storedReason == nil || *storedReason != reason {
		t.Errorf("expected reason=%q, got %v", reason, storedReason)
	}

	// Push subs gone.
	var pushCount int
	_ = db.Get(&pushCount, "SELECT COUNT(*) FROM push_subscriptions WHERE user_id = $1", u.ID)
	if pushCount != 0 {
		t.Errorf("expected push subs to be wiped, got %d", pushCount)
	}

	// Refresh tokens revoked.
	var revokedCount int
	_ = db.Get(&revokedCount,
		"SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1 AND revoked_at IS NOT NULL",
		u.ID,
	)
	if revokedCount != 1 {
		t.Errorf("expected 1 revoked refresh token, got %d", revokedCount)
	}
}

// TestDeleteMyAccount_RejectsWrongPassword — bcrypt mismatch returns
// Unauthenticated. deleted_at stays NULL.
func TestDeleteMyAccount_RejectsWrongPassword(t *testing.T) {
	db := setupTestDB(t)
	t.Setenv("API_SECRET", "integration-test-secret")

	u := seedTestUser(t, db, "keepme", "keep@example.com", "TestPass123")
	handler := &UserHandler{DB: db}
	ctx := httpctx.WithUserID(context.Background(), u.ID)

	_, err := handler.DeleteMyAccount(ctx, connectReq(&userv1.DeleteMyAccountRequest{
		Password: "TotallyWrong",
	}, ""))
	assertConnectError(t, err, connect.CodeUnauthenticated)

	var deletedAt *time.Time
	_ = db.Get(&deletedAt, "SELECT deleted_at FROM users WHERE id = $1", u.ID)
	if deletedAt != nil {
		t.Error("deleted_at should still be NULL after failed attempt")
	}
}

// TestDeleteMyAccount_RejectsAnon — no auth context → Unauthenticated.
func TestDeleteMyAccount_RejectsAnon(t *testing.T) {
	db := setupTestDB(t)
	handler := &UserHandler{DB: db}
	_, err := handler.DeleteMyAccount(context.Background(), connectReq(&userv1.DeleteMyAccountRequest{
		Password: "whatever",
	}, ""))
	assertConnectError(t, err, connect.CodeUnauthenticated)
}

// TestLogin_RejectsDeletedUser — after soft-delete, the email/password
// combo no longer works. Surfaced as "incorrect email or password" so
// we don't leak which accounts are banned.
func TestLogin_RejectsDeletedUser(t *testing.T) {
	db := setupTestDB(t)
	t.Setenv("API_SECRET", "integration-test-secret")

	u := seedTestUser(t, db, "banned", "banned@example.com", "TestPass123")

	// Soft-delete directly (bypass the RPC — this test exercises
	// the Login side specifically).
	if _, err := db.Exec("UPDATE users SET deleted_at = NOW() WHERE id = $1", u.ID); err != nil {
		t.Fatalf("soft-delete: %v", err)
	}

	authHandler := &AuthHandler{DB: db}
	_, err := authHandler.Login(context.Background(), connectReq(&authv1.LoginRequest{
		Email:    "banned@example.com",
		Password: "TestPass123",
	}, ""))
	assertConnectError(t, err, connect.CodeUnauthenticated)
}

// TestHardDeleteExpiredAccounts_SweepsPastRetention — the daily cron
// target. Seed a user with an artificially-old deleted_at, temporarily
// shorten the retention window so the cron "sees" them as expired,
// run the cron, verify the row + cascaded engagement is gone.
func TestHardDeleteExpiredAccounts_SweepsPastRetention(t *testing.T) {
	db := setupTestDB(t)

	// Three users: one soft-deleted far in the past (should sweep),
	// one soft-deleted just now (should NOT sweep), one never
	// deleted (should NOT sweep).
	past := seedTestUser(t, db, "gone", "gone@example.com", "TestPass123")
	recent := seedTestUser(t, db, "leaving", "leaving@example.com", "TestPass123")
	alive := seedTestUser(t, db, "here", "here@example.com", "TestPass123")

	if _, err := db.Exec(
		"UPDATE users SET deleted_at = NOW() - INTERVAL '100 days' WHERE id = $1", past.ID,
	); err != nil {
		t.Fatalf("age past user: %v", err)
	}
	if _, err := db.Exec(
		"UPDATE users SET deleted_at = NOW() WHERE id = $1", recent.ID,
	); err != nil {
		t.Fatalf("age recent user: %v", err)
	}

	// Seed some engagement on the past user so we can verify the
	// cascade wipes it. Reuse the helper — seedTestMatchup creates
	// both the matchup + two child items.
	m := seedTestMatchup(t, db, past.ID, "Leaving a trail")
	_ = m

	n, err := HardDeleteExpiredAccounts(context.Background(), db)
	if err != nil {
		t.Fatalf("HardDeleteExpiredAccounts: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 hard-deleted user, got %d", n)
	}

	// past user row gone; recent + alive still there.
	var pastCount, recentCount, aliveCount int
	_ = db.Get(&pastCount, "SELECT COUNT(*) FROM users WHERE id = $1", past.ID)
	_ = db.Get(&recentCount, "SELECT COUNT(*) FROM users WHERE id = $1", recent.ID)
	_ = db.Get(&aliveCount, "SELECT COUNT(*) FROM users WHERE id = $1", alive.ID)
	if pastCount != 0 {
		t.Error("past user should be hard-deleted")
	}
	if recentCount != 1 {
		t.Error("recent user should NOT be hard-deleted yet")
	}
	if aliveCount != 1 {
		t.Error("alive user should NOT be touched")
	}

	// Engagement cascade: past user's matchup gone.
	var matchupCount int
	_ = db.Get(&matchupCount, "SELECT COUNT(*) FROM matchups WHERE author_id = $1", past.ID)
	if matchupCount != 0 {
		t.Errorf("expected past user's matchups to cascade-delete, got %d remaining", matchupCount)
	}
}
