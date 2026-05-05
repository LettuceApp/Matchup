//go:build integration

package controllers

import (
	"context"
	"testing"
	"time"

	authv1 "Matchup/gen/auth/v1"
	bracketv1 "Matchup/gen/bracket/v1"
	commentv1 "Matchup/gen/comment/v1"
	matchupv1 "Matchup/gen/matchup/v1"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
)

// Email verification integration tests. Cover:
//   - mintEmailVerificationToken + ConfirmEmailVerification happy path
//   - expired tokens rejected
//   - already-used tokens rejected
//   - double-confirm rejected under a single-use guard
//   - soft-gate blocks unverified users on content-create paths
//   - soft-gate allows own-matchup commenting without verification

func TestConfirmEmailVerification_StampsUser(t *testing.T) {
	db := setupTestDB(t)
	u := seedUnverifiedUser(t, db, "alice", "alice@example.com", "TestPass123")

	token, err := mintEmailVerificationToken(context.Background(), db, u.ID)
	if err != nil {
		t.Fatalf("mintEmailVerificationToken: %v", err)
	}

	h := &AuthHandler{DB: db}
	if _, err := h.ConfirmEmailVerification(context.Background(), connectReq(&authv1.ConfirmEmailVerificationRequest{
		Token: token,
	}, "")); err != nil {
		t.Fatalf("ConfirmEmailVerification: %v", err)
	}

	var verifiedAt *string
	_ = db.QueryRow("SELECT email_verified_at::text FROM users WHERE id = $1", u.ID).Scan(&verifiedAt)
	if verifiedAt == nil {
		t.Fatal("expected email_verified_at to be stamped")
	}

	// Token row should now carry used_at.
	var usedAt *string
	_ = db.QueryRow(
		"SELECT used_at::text FROM email_verification_tokens WHERE token_hash = $1",
		hashEmailVerificationToken(token),
	).Scan(&usedAt)
	if usedAt == nil {
		t.Error("expected used_at to be stamped on the token row")
	}
}

func TestConfirmEmailVerification_ExpiredTokenRejected(t *testing.T) {
	db := setupTestDB(t)
	u := seedUnverifiedUser(t, db, "alice", "alice@example.com", "TestPass123")

	// Mint normally, then push expires_at into the past so the
	// handler's expiry branch fires.
	token, err := mintEmailVerificationToken(context.Background(), db, u.ID)
	if err != nil {
		t.Fatalf("mintEmailVerificationToken: %v", err)
	}
	if _, err := db.Exec(
		"UPDATE email_verification_tokens SET expires_at = $1 WHERE token_hash = $2",
		time.Now().Add(-1*time.Hour), hashEmailVerificationToken(token),
	); err != nil {
		t.Fatalf("backdate expires_at: %v", err)
	}

	h := &AuthHandler{DB: db}
	_, err = h.ConfirmEmailVerification(context.Background(), connectReq(&authv1.ConfirmEmailVerificationRequest{
		Token: token,
	}, ""))
	assertConnectError(t, err, connect.CodeInvalidArgument)

	// User row stays unverified.
	var verifiedAt *string
	_ = db.QueryRow("SELECT email_verified_at::text FROM users WHERE id = $1", u.ID).Scan(&verifiedAt)
	if verifiedAt != nil {
		t.Errorf("expected email_verified_at to stay NULL after rejected confirm, got %v", *verifiedAt)
	}
}

func TestConfirmEmailVerification_SingleUse(t *testing.T) {
	db := setupTestDB(t)
	u := seedUnverifiedUser(t, db, "alice", "alice@example.com", "TestPass123")

	token, err := mintEmailVerificationToken(context.Background(), db, u.ID)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	h := &AuthHandler{DB: db}
	if _, err := h.ConfirmEmailVerification(context.Background(), connectReq(&authv1.ConfirmEmailVerificationRequest{
		Token: token,
	}, "")); err != nil {
		t.Fatalf("first confirm: %v", err)
	}
	// Second call with same token — already used.
	_, err = h.ConfirmEmailVerification(context.Background(), connectReq(&authv1.ConfirmEmailVerificationRequest{
		Token: token,
	}, ""))
	assertConnectError(t, err, connect.CodeInvalidArgument)
}

func TestRequestEmailVerification_NoOpForVerifiedUser(t *testing.T) {
	db := setupTestDB(t)
	u := seedTestUser(t, db, "alice", "alice@example.com", "TestPass123")
	if _, err := db.Exec("UPDATE users SET email_verified_at = NOW() WHERE id = $1", u.ID); err != nil {
		t.Fatalf("pre-verify user: %v", err)
	}

	h := &AuthHandler{DB: db}
	ctx := httpctx.WithUserID(context.Background(), u.ID)
	resp, err := h.RequestEmailVerification(ctx, connectReq(&authv1.RequestEmailVerificationRequest{}, ""))
	if err != nil {
		t.Fatalf("RequestEmailVerification: %v", err)
	}
	if resp.Msg.Message == "" {
		t.Error("expected a friendly message for already-verified user")
	}

	// No new token row should have been minted.
	var count int
	_ = db.Get(&count, "SELECT COUNT(*) FROM email_verification_tokens WHERE user_id = $1", u.ID)
	if count != 0 {
		t.Errorf("expected 0 tokens for already-verified user, got %d", count)
	}
}

// TestSoftGate_BlocksUnverifiedCreateMatchup — unverified users hit
// FailedPrecondition when creating a matchup.
func TestSoftGate_BlocksUnverifiedCreateMatchup(t *testing.T) {
	db := setupTestDB(t)
	u := seedUnverifiedUser(t, db, "alice", "alice@example.com", "TestPass123")

	h := &MatchupHandler{DB: db}
	ctx := httpctx.WithUserID(context.Background(), u.ID)

	_, err := h.CreateMatchup(ctx, connectReq(&matchupv1.CreateMatchupRequest{
		Title: "Trying to create before verifying",
	}, ""))
	assertConnectError(t, err, connect.CodeFailedPrecondition)
}

// TestSoftGate_AllowsVerifiedCreateMatchup — once verified, the same
// call succeeds. This is the positive-path for the gate's logic.
func TestSoftGate_AllowsVerifiedCreateMatchup(t *testing.T) {
	db := setupTestDB(t)
	u := seedTestUser(t, db, "alice", "alice@example.com", "TestPass123")
	if _, err := db.Exec("UPDATE users SET email_verified_at = NOW() WHERE id = $1", u.ID); err != nil {
		t.Fatalf("pre-verify user: %v", err)
	}

	h := &MatchupHandler{DB: db}
	ctx := httpctx.WithUserID(context.Background(), u.ID)

	// CreateMatchup normally needs items etc.; the gate fires BEFORE
	// validation, so FailedPrecondition absence is the assertion.
	_, err := h.CreateMatchup(ctx, connectReq(&matchupv1.CreateMatchupRequest{
		Title: "Verified user",
	}, ""))
	if err != nil {
		if connectErr, ok := err.(*connect.Error); ok && connectErr.Code() == connect.CodeFailedPrecondition {
			t.Errorf("verified user should NOT hit FailedPrecondition; got: %v", err)
		}
		// Any other error (missing items, missing duration) is fine —
		// it means we got past the soft-gate into real validation.
	}
}

// TestSoftGate_BlocksUnverifiedCreateBracket — parallel to the matchup
// test. Bracket creation is the other high-fanout spam surface.
func TestSoftGate_BlocksUnverifiedCreateBracket(t *testing.T) {
	db := setupTestDB(t)
	u := seedUnverifiedUser(t, db, "alice", "alice@example.com", "TestPass123")

	h := &BracketHandler{DB: db}
	ctx := httpctx.WithUserID(context.Background(), u.ID)

	_, err := h.CreateBracket(ctx, connectReq(&bracketv1.CreateBracketRequest{
		Title: "Bracket spam attempt",
		Size:  4,
	}, ""))
	assertConnectError(t, err, connect.CodeFailedPrecondition)
}

// TestSoftGate_AllowsOwnMatchupComment — the gate is deliberately
// narrowed to "commenting on someone else's content". A user
// commenting on their own matchup should succeed whether or not
// they're verified — otherwise a fresh signup can't even reply to
// @mentions on their own post.
func TestSoftGate_AllowsOwnMatchupComment(t *testing.T) {
	db := setupTestDB(t)
	author := seedUnverifiedUser(t, db, "alice", "alice@example.com", "TestPass123")
	// Author is NOT verified. Seed a matchup authored by them.
	m := seedTestMatchup(t, db, author.ID, "My own post")
	if _, err := db.Exec("UPDATE matchups SET status = 'active' WHERE id = $1", m.ID); err != nil {
		t.Fatalf("activate matchup: %v", err)
	}

	h := &CommentHandler{DB: db, ReadDB: db}
	ctx := httpctx.WithUserID(context.Background(), author.ID)

	if _, err := h.CreateComment(ctx, connectReq(&commentv1.CreateCommentRequest{
		MatchupId: m.PublicID,
		Body:      "first",
	}, "")); err != nil {
		t.Errorf("author should be able to comment on own matchup without verification; got %v", err)
	}
}

// TestSoftGate_BlocksCommentingOnOthers — unverified user tries to
// comment on someone else's matchup → FailedPrecondition.
func TestSoftGate_BlocksCommentingOnOthers(t *testing.T) {
	db := setupTestDB(t)
	author := seedTestUser(t, db, "alice", "alice@example.com", "TestPass123")
	commenter := seedUnverifiedUser(t, db, "bob", "bob@example.com", "TestPass123")
	m := seedTestMatchup(t, db, author.ID, "Not yours")
	if _, err := db.Exec("UPDATE matchups SET status = 'active' WHERE id = $1", m.ID); err != nil {
		t.Fatalf("activate matchup: %v", err)
	}

	h := &CommentHandler{DB: db, ReadDB: db}
	ctx := httpctx.WithUserID(context.Background(), commenter.ID)

	_, err := h.CreateComment(ctx, connectReq(&commentv1.CreateCommentRequest{
		MatchupId: m.PublicID,
		Body:      "drive-by",
	}, ""))
	assertConnectError(t, err, connect.CodeFailedPrecondition)
}
