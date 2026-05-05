//go:build integration

package controllers

import (
	"context"
	"testing"

	activityv1 "Matchup/gen/activity/v1"
	userv1 "Matchup/gen/user/v1"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
)

// TestBlockUser_SeversFollowBothWaysAndCountsBothDirections — the
// heaviest logic in the handler. Pre-condition: A follows B and B
// follows A. Block A→B → both follow rows gone + both users'
// counters decremented + user_blocks row present.
func TestBlockUser_SeversFollowBothWaysAndCountsBothDirections(t *testing.T) {
	db := setupTestDB(t)
	a := seedTestUser(t, db, "alice", "a@example.com", "TestPass123")
	b := seedTestUser(t, db, "bob", "b@example.com", "TestPass123")

	// Seed the mutual follow + bump counters to match. seedTestUser
	// sets both counts to 0 on creation, so we update manually here.
	for _, pair := range []struct{ from, to uint }{{a.ID, b.ID}, {b.ID, a.ID}} {
		if _, err := db.Exec(
			"INSERT INTO follows (follower_id, followed_id, created_at) VALUES ($1, $2, NOW())",
			pair.from, pair.to,
		); err != nil {
			t.Fatalf("seed follow: %v", err)
		}
	}
	if _, err := db.Exec(
		"UPDATE users SET following_count = 1, followers_count = 1 WHERE id IN ($1, $2)",
		a.ID, b.ID,
	); err != nil {
		t.Fatalf("seed counts: %v", err)
	}

	h := &UserHandler{DB: db}
	ctx := httpctx.WithUserID(context.Background(), a.ID)
	_, err := h.BlockUser(ctx, connectReq(&userv1.BlockUserRequest{
		Id: b.Username,
	}, ""))
	if err != nil {
		t.Fatalf("BlockUser: %v", err)
	}

	// Block row exists.
	var blockCount int
	_ = db.Get(&blockCount,
		"SELECT COUNT(*) FROM user_blocks WHERE blocker_id = $1 AND blocked_id = $2",
		a.ID, b.ID,
	)
	if blockCount != 1 {
		t.Errorf("expected 1 block row, got %d", blockCount)
	}

	// Follow edges gone both ways.
	var followCount int
	_ = db.Get(&followCount,
		"SELECT COUNT(*) FROM follows WHERE (follower_id = $1 AND followed_id = $2) OR (follower_id = $2 AND followed_id = $1)",
		a.ID, b.ID,
	)
	if followCount != 0 {
		t.Errorf("expected 0 follow edges, got %d", followCount)
	}

	// Counters decremented to 0 on both sides.
	var aFollowing, aFollowers, bFollowing, bFollowers int64
	_ = db.QueryRow(
		"SELECT following_count, followers_count FROM users WHERE id = $1", a.ID,
	).Scan(&aFollowing, &aFollowers)
	_ = db.QueryRow(
		"SELECT following_count, followers_count FROM users WHERE id = $1", b.ID,
	).Scan(&bFollowing, &bFollowers)
	if aFollowing != 0 || aFollowers != 0 || bFollowing != 0 || bFollowers != 0 {
		t.Errorf("counters after block: a=(f:%d, fr:%d) b=(f:%d, fr:%d); expected all 0",
			aFollowing, aFollowers, bFollowing, bFollowers)
	}
}

// TestBlockUser_Idempotent — blocking twice doesn't error. ON CONFLICT
// DO NOTHING on user_blocks is the guard.
func TestBlockUser_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	a := seedTestUser(t, db, "alice", "a@example.com", "TestPass123")
	b := seedTestUser(t, db, "bob", "b@example.com", "TestPass123")

	h := &UserHandler{DB: db}
	ctx := httpctx.WithUserID(context.Background(), a.ID)
	for i := 0; i < 2; i++ {
		if _, err := h.BlockUser(ctx, connectReq(&userv1.BlockUserRequest{Id: b.Username}, "")); err != nil {
			t.Fatalf("BlockUser call %d: %v", i, err)
		}
	}
	var count int
	_ = db.Get(&count, "SELECT COUNT(*) FROM user_blocks WHERE blocker_id = $1 AND blocked_id = $2", a.ID, b.ID)
	if count != 1 {
		t.Errorf("expected 1 row after idempotent block, got %d", count)
	}
}

// TestBlockUser_RejectsSelfBlock — handler returns InvalidArgument.
func TestBlockUser_RejectsSelfBlock(t *testing.T) {
	db := setupTestDB(t)
	a := seedTestUser(t, db, "alice", "a@example.com", "TestPass123")

	h := &UserHandler{DB: db}
	ctx := httpctx.WithUserID(context.Background(), a.ID)
	_, err := h.BlockUser(ctx, connectReq(&userv1.BlockUserRequest{Id: a.Username}, ""))
	assertConnectError(t, err, connect.CodeInvalidArgument)
}

// TestBlockUser_DeletesExistingMute — block supersedes mute. If A had
// muted B, then A blocks B, the mute row goes away.
func TestBlockUser_DeletesExistingMute(t *testing.T) {
	db := setupTestDB(t)
	a := seedTestUser(t, db, "alice", "a@example.com", "TestPass123")
	b := seedTestUser(t, db, "bob", "b@example.com", "TestPass123")

	// Seed a mute first.
	if _, err := db.Exec(
		"INSERT INTO user_mutes (muter_id, muted_id) VALUES ($1, $2)",
		a.ID, b.ID,
	); err != nil {
		t.Fatalf("seed mute: %v", err)
	}

	h := &UserHandler{DB: db}
	ctx := httpctx.WithUserID(context.Background(), a.ID)
	if _, err := h.BlockUser(ctx, connectReq(&userv1.BlockUserRequest{Id: b.Username}, "")); err != nil {
		t.Fatalf("BlockUser: %v", err)
	}

	var muteCount int
	_ = db.Get(&muteCount, "SELECT COUNT(*) FROM user_mutes WHERE muter_id = $1 AND muted_id = $2", a.ID, b.ID)
	if muteCount != 0 {
		t.Errorf("expected mute to be cleared by block, got %d", muteCount)
	}
}

// TestUnblockUser_RemovesRow — plain DELETE. No auto-restore of
// follows (those stay severed until users manually re-follow).
func TestUnblockUser_RemovesRow(t *testing.T) {
	db := setupTestDB(t)
	a := seedTestUser(t, db, "alice", "a@example.com", "TestPass123")
	b := seedTestUser(t, db, "bob", "b@example.com", "TestPass123")

	if _, err := db.Exec(
		"INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)",
		a.ID, b.ID,
	); err != nil {
		t.Fatalf("seed block: %v", err)
	}

	h := &UserHandler{DB: db}
	ctx := httpctx.WithUserID(context.Background(), a.ID)
	if _, err := h.UnblockUser(ctx, connectReq(&userv1.UnblockUserRequest{Id: b.Username}, "")); err != nil {
		t.Fatalf("UnblockUser: %v", err)
	}

	var count int
	_ = db.Get(&count, "SELECT COUNT(*) FROM user_blocks WHERE blocker_id = $1 AND blocked_id = $2", a.ID, b.ID)
	if count != 0 {
		t.Errorf("expected 0 block rows after unblock, got %d", count)
	}
}

// TestMuteUser_OneWayInsertAndIdempotent — mute inserts the row
// without touching follows; doubling up is a no-op.
func TestMuteUser_OneWayInsertAndIdempotent(t *testing.T) {
	db := setupTestDB(t)
	a := seedTestUser(t, db, "alice", "a@example.com", "TestPass123")
	b := seedTestUser(t, db, "bob", "b@example.com", "TestPass123")

	// Seed a follow edge both directions so we can verify mute
	// DOESN'T touch them (the contrast with block).
	for _, pair := range []struct{ from, to uint }{{a.ID, b.ID}, {b.ID, a.ID}} {
		if _, err := db.Exec(
			"INSERT INTO follows (follower_id, followed_id, created_at) VALUES ($1, $2, NOW())",
			pair.from, pair.to,
		); err != nil {
			t.Fatalf("seed follow: %v", err)
		}
	}

	h := &UserHandler{DB: db}
	ctx := httpctx.WithUserID(context.Background(), a.ID)
	for i := 0; i < 2; i++ {
		if _, err := h.MuteUser(ctx, connectReq(&userv1.MuteUserRequest{Id: b.Username}, "")); err != nil {
			t.Fatalf("MuteUser call %d: %v", i, err)
		}
	}

	var muteCount int
	_ = db.Get(&muteCount, "SELECT COUNT(*) FROM user_mutes WHERE muter_id = $1 AND muted_id = $2", a.ID, b.ID)
	if muteCount != 1 {
		t.Errorf("expected 1 mute row after idempotent mutes, got %d", muteCount)
	}

	// Follows still there — mute is purely passive.
	var followCount int
	_ = db.Get(&followCount,
		"SELECT COUNT(*) FROM follows WHERE (follower_id = $1 AND followed_id = $2) OR (follower_id = $2 AND followed_id = $1)",
		a.ID, b.ID,
	)
	if followCount != 2 {
		t.Errorf("expected 2 follow rows to remain after mute, got %d", followCount)
	}
}

// TestListBlocks_ReturnsPaginatedUsers — happy path; asserts the
// blocked user shows up + the cursor logic doesn't crash when the
// list fits in one page.
func TestListBlocks_ReturnsPaginatedUsers(t *testing.T) {
	db := setupTestDB(t)
	a := seedTestUser(t, db, "alice", "a@example.com", "TestPass123")
	b := seedTestUser(t, db, "bob", "b@example.com", "TestPass123")
	c := seedTestUser(t, db, "carol", "c@example.com", "TestPass123")

	for _, tid := range []uint{b.ID, c.ID} {
		if _, err := db.Exec(
			"INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)",
			a.ID, tid,
		); err != nil {
			t.Fatalf("seed block: %v", err)
		}
	}

	h := &UserHandler{DB: db}
	ctx := httpctx.WithUserID(context.Background(), a.ID)
	resp, err := h.ListBlocks(ctx, connectReq(&userv1.ListBlocksRequest{}, ""))
	if err != nil {
		t.Fatalf("ListBlocks: %v", err)
	}
	if len(resp.Msg.Users) != 2 {
		t.Errorf("expected 2 blocked users, got %d", len(resp.Msg.Users))
	}
}

// TestGetUserActivity_FiltersBlockedUserItems — activity from a user the
// viewer has blocked must disappear from the feed. The filter runs
// post-fan-out on actor_username, which is case-insensitive — so this
// also guards against username casing drift.
//
// Seed shape:
//   - viewer owns a matchup
//   - blocked user likes it  → like_received row lands in viewer's feed
//   - blocked user follows viewer → new_follower row lands too
//   - Then: viewer blocks the user. Both rows must disappear.
func TestGetUserActivity_FiltersBlockedUserItems(t *testing.T) {
	db := setupTestDB(t)
	viewer := seedTestUser(t, db, "viewer", "v@example.com", "TestPass123")
	blocked := seedTestUser(t, db, "baduser", "b@example.com", "TestPass123")
	m := seedTestMatchup(t, db, viewer.ID, "Mine")

	// Seed a like + a follow from blocked user to viewer.
	if _, err := db.Exec(
		`INSERT INTO likes (user_id, matchup_id, created_at, updated_at) VALUES ($1, $2, NOW(), NOW())`,
		blocked.ID, m.ID,
	); err != nil {
		t.Fatalf("seed like: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO follows (follower_id, followed_id, created_at) VALUES ($1, $2, NOW())`,
		blocked.ID, viewer.ID,
	); err != nil {
		t.Fatalf("seed follow: %v", err)
	}

	ah := &ActivityHandler{DB: db, ReadDB: db}
	ctx := httpctx.WithUserID(context.Background(), viewer.ID)

	// Pre-block baseline: both kinds surface.
	before, err := ah.GetUserActivity(ctx, connectReq(&activityv1.GetUserActivityRequest{
		UserId: viewer.Username,
	}, ""))
	if err != nil {
		t.Fatalf("pre-block GetUserActivity: %v", err)
	}
	beforeKinds := map[string]int{}
	for _, it := range before.Msg.Items {
		beforeKinds[it.Kind]++
	}
	if beforeKinds["like_received"] < 1 || beforeKinds["new_follower"] < 1 {
		t.Fatalf("expected like_received + new_follower before block; got %v", beforeKinds)
	}

	// Block. Note: BlockUser also severs the follow edge, so
	// new_follower naturally goes away at the source. But the like row
	// stays (mute/block doesn't retroactively delete likes) and the
	// filter is what must hide it.
	uh := &UserHandler{DB: db}
	if _, err := uh.BlockUser(ctx, connectReq(&userv1.BlockUserRequest{Id: blocked.Username}, "")); err != nil {
		t.Fatalf("BlockUser: %v", err)
	}

	after, err := ah.GetUserActivity(ctx, connectReq(&activityv1.GetUserActivityRequest{
		UserId: viewer.Username,
	}, ""))
	if err != nil {
		t.Fatalf("post-block GetUserActivity: %v", err)
	}
	for _, it := range after.Msg.Items {
		if it.ActorUsername != nil && *it.ActorUsername == "baduser" {
			t.Errorf("blocked actor still surfacing post-block: kind=%s actor=%q",
				it.Kind, *it.ActorUsername)
		}
	}
}

// TestGetUserActivity_FiltersMutedUserItems — mute is one-way. A muted
// user's activity hides from the muter's feed without severing follows
// or likes at the source.
func TestGetUserActivity_FiltersMutedUserItems(t *testing.T) {
	db := setupTestDB(t)
	viewer := seedTestUser(t, db, "viewer", "v@example.com", "TestPass123")
	muted := seedTestUser(t, db, "noisy", "n@example.com", "TestPass123")
	m := seedTestMatchup(t, db, viewer.ID, "Mine")

	if _, err := db.Exec(
		`INSERT INTO likes (user_id, matchup_id, created_at, updated_at) VALUES ($1, $2, NOW(), NOW())`,
		muted.ID, m.ID,
	); err != nil {
		t.Fatalf("seed like: %v", err)
	}

	uh := &UserHandler{DB: db}
	ctx := httpctx.WithUserID(context.Background(), viewer.ID)
	if _, err := uh.MuteUser(ctx, connectReq(&userv1.MuteUserRequest{Id: muted.Username}, "")); err != nil {
		t.Fatalf("MuteUser: %v", err)
	}

	ah := &ActivityHandler{DB: db, ReadDB: db}
	resp, err := ah.GetUserActivity(ctx, connectReq(&activityv1.GetUserActivityRequest{
		UserId: viewer.Username,
	}, ""))
	if err != nil {
		t.Fatalf("GetUserActivity: %v", err)
	}
	for _, it := range resp.Msg.Items {
		if it.ActorUsername != nil && *it.ActorUsername == "noisy" {
			t.Errorf("muted actor still surfacing: kind=%s", it.Kind)
		}
	}

	// Like row remains in DB (mute is passive, not destructive).
	var likeCount int
	_ = db.Get(&likeCount,
		"SELECT COUNT(*) FROM likes WHERE user_id = $1 AND matchup_id = $2",
		muted.ID, m.ID,
	)
	if likeCount != 1 {
		t.Errorf("expected mute to leave likes untouched, got %d", likeCount)
	}
}

// TestGetUser_BlockedEitherDirectionReturns404 — if either side of the
// block edge is set, GetUser on the opposite user returns NotFound.
// Prevents a blocked user from surveilling the blocker's profile.
func TestGetUser_BlockedEitherDirectionReturns404(t *testing.T) {
	db := setupTestDB(t)
	a := seedTestUser(t, db, "alice", "a@example.com", "TestPass123")
	b := seedTestUser(t, db, "bob", "b@example.com", "TestPass123")

	if _, err := db.Exec(
		"INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)",
		a.ID, b.ID,
	); err != nil {
		t.Fatalf("seed block: %v", err)
	}

	h := &UserHandler{DB: db, ReadDB: db}

	// a → looking up b: hidden.
	ctxA := httpctx.WithUserID(context.Background(), a.ID)
	_, err := h.GetUser(ctxA, connectReq(&userv1.GetUserRequest{Id: b.Username}, ""))
	assertConnectError(t, err, connect.CodeNotFound)

	// b → looking up a: also hidden (bidirectional block).
	ctxB := httpctx.WithUserID(context.Background(), b.ID)
	_, err = h.GetUser(ctxB, connectReq(&userv1.GetUserRequest{Id: a.Username}, ""))
	assertConnectError(t, err, connect.CodeNotFound)

	// a → looking up self: always allowed (self-lookup bypass).
	if _, err := h.GetUser(ctxA, connectReq(&userv1.GetUserRequest{Id: a.Username}, "")); err != nil {
		t.Errorf("self-lookup should never 404 on block; got %v", err)
	}
}

// TestGetUser_AnonCanSeePublicProfile — public accounts are visible
// to anonymous viewers (no token + no httpctx user). Confirms the
// new anon-friendly route works end-to-end.
func TestGetUser_AnonCanSeePublicProfile(t *testing.T) {
	db := setupTestDB(t)
	target := seedTestUser(t, db, "publicly", "p@example.com", "TestPass123")

	h := &UserHandler{DB: db, ReadDB: db}
	resp, err := h.GetUser(context.Background(), connectReq(&userv1.GetUserRequest{
		Id: target.Username,
	}, ""))
	if err != nil {
		t.Fatalf("GetUser anon → public: %v", err)
	}
	if resp.Msg.User.GetUsername() != target.Username {
		t.Errorf("expected username=%q, got %q", target.Username, resp.Msg.User.GetUsername())
	}
}

// TestGetUser_AnonRejectedFromPrivateProfile — private accounts return
// 404 to anon callers. Same shape as a missing user — the existence
// of the account isn't leaked.
func TestGetUser_AnonRejectedFromPrivateProfile(t *testing.T) {
	db := setupTestDB(t)
	target := seedTestUser(t, db, "secret", "s@example.com", "TestPass123")
	if _, err := db.Exec("UPDATE users SET is_private = true WHERE id = $1", target.ID); err != nil {
		t.Fatalf("flip is_private: %v", err)
	}

	h := &UserHandler{DB: db, ReadDB: db}
	_, err := h.GetUser(context.Background(), connectReq(&userv1.GetUserRequest{
		Id: target.Username,
	}, ""))
	assertConnectError(t, err, connect.CodeNotFound)
}

// TestGetUser_AuthedCanSeePrivateProfile — once signed in, the anon-
// only gate doesn't fire. Downstream visibility helpers
// (canViewUserContent on matchup reads) still gate followers-only
// content — that's the right layer for content-level enforcement.
func TestGetUser_AuthedCanSeePrivateProfile(t *testing.T) {
	db := setupTestDB(t)
	viewer := seedTestUser(t, db, "viewer", "v@example.com", "TestPass123")
	target := seedTestUser(t, db, "secret", "s@example.com", "TestPass123")
	if _, err := db.Exec("UPDATE users SET is_private = true WHERE id = $1", target.ID); err != nil {
		t.Fatalf("flip is_private: %v", err)
	}

	h := &UserHandler{DB: db, ReadDB: db}
	ctx := httpctx.WithUserID(context.Background(), viewer.ID)
	if _, err := h.GetUser(ctx, connectReq(&userv1.GetUserRequest{
		Id: target.Username,
	}, "")); err != nil {
		t.Errorf("authed viewer should see private profile shell; got %v", err)
	}
}
