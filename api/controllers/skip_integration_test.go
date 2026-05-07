//go:build integration

package controllers

import (
	"context"
	"testing"

	matchupv1 "Matchup/gen/matchup/v1"

	"connectrpc.com/connect"
)

// Skip / can't-decide vote tests.
//
// Skips live in matchup_votes alongside picks but with kind='skip' and
// a NULL matchup_item_public_id. Migration 025 added the column +
// nullability + check constraints. Skips do NOT count toward the anon
// vote cap and do NOT increment any item's votes counter.

const skipAnonID = "test-skip-anon-uuid-7777"

// TestSkipMatchup_AnonInsertsSkipRow — first call lands a row with
// kind='skip' and a NULL matchup_item_public_id. Returns
// AlreadySkipped=false.
func TestSkipMatchup_AnonInsertsSkipRow(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "alice", "alice@example.com", "TestPass123")
	m := seedTestMatchup(t, db, owner.ID, "skippable")

	h := &MatchupItemHandler{DB: db, ReadDB: db}
	anonID := skipAnonID

	resp, err := h.SkipMatchup(context.Background(), connectReq(&matchupv1.SkipMatchupRequest{
		MatchupId: m.PublicID,
		AnonId:    &anonID,
	}, ""))
	if err != nil {
		t.Fatalf("skip: %v", err)
	}
	if resp.Msg.AlreadySkipped {
		t.Errorf("AlreadySkipped = true, want false on first call")
	}

	var row struct {
		Kind                string  `db:"kind"`
		MatchupItemPublicID *string `db:"matchup_item_public_id"`
	}
	if err := db.Get(&row,
		"SELECT kind, matchup_item_public_id FROM matchup_votes WHERE anon_id = $1 AND matchup_public_id = $2",
		anonID, m.PublicID,
	); err != nil {
		t.Fatalf("fetch skip row: %v", err)
	}
	if row.Kind != "skip" {
		t.Errorf("kind = %q, want %q", row.Kind, "skip")
	}
	if row.MatchupItemPublicID != nil {
		t.Errorf("matchup_item_public_id = %v, want nil for skip rows", *row.MatchupItemPublicID)
	}
}

// TestSkipMatchup_AnonIdempotent — second skip on the same matchup
// returns AlreadySkipped=true and doesn't insert another row.
func TestSkipMatchup_AnonIdempotent(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "alice", "alice@example.com", "TestPass123")
	m := seedTestMatchup(t, db, owner.ID, "double-skip")

	h := &MatchupItemHandler{DB: db, ReadDB: db}
	anonID := skipAnonID

	for i := 0; i < 2; i++ {
		resp, err := h.SkipMatchup(context.Background(), connectReq(&matchupv1.SkipMatchupRequest{
			MatchupId: m.PublicID,
			AnonId:    &anonID,
		}, ""))
		if err != nil {
			t.Fatalf("skip %d: %v", i+1, err)
		}
		want := i == 1
		if resp.Msg.AlreadySkipped != want {
			t.Errorf("call %d AlreadySkipped = %v, want %v", i+1, resp.Msg.AlreadySkipped, want)
		}
	}

	var n int
	_ = db.Get(&n, "SELECT COUNT(*) FROM matchup_votes WHERE anon_id = $1", anonID)
	if n != 1 {
		t.Errorf("expected 1 row after duplicate skip, got %d", n)
	}
}

// TestSkipMatchup_AnonDoesNotCountTowardCap — burn the 3-vote cap on
// picks, then skip several more matchups. Skips should land cleanly
// without tripping the cap.
func TestSkipMatchup_AnonDoesNotCountTowardCap(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "alice", "alice@example.com", "TestPass123")

	h := &MatchupItemHandler{DB: db, ReadDB: db}
	anonID := skipAnonID

	// Burn all 3 picks.
	for i := 0; i < 3; i++ {
		m := seedTestMatchup(t, db, owner.ID, "pick")
		itemID := firstItemPublicID(t, db, m.ID)
		if _, err := h.VoteItem(context.Background(), connectReq(&matchupv1.VoteItemRequest{
			Id:     itemID,
			AnonId: &anonID,
		}, "")); err != nil {
			t.Fatalf("seed pick %d: %v", i+1, err)
		}
	}

	// Now skip 5 more matchups. None should error.
	for i := 0; i < 5; i++ {
		m := seedTestMatchup(t, db, owner.ID, "skip-after-cap")
		if _, err := h.SkipMatchup(context.Background(), connectReq(&matchupv1.SkipMatchupRequest{
			MatchupId: m.PublicID,
			AnonId:    &anonID,
		}, "")); err != nil {
			t.Fatalf("post-cap skip %d: %v", i+1, err)
		}
	}

	// Total rows: 3 picks + 5 skips = 8. Cap counter only sees picks.
	var picks, skips int
	_ = db.Get(&picks, "SELECT COUNT(*) FROM matchup_votes WHERE anon_id = $1 AND kind = 'pick'", anonID)
	_ = db.Get(&skips, "SELECT COUNT(*) FROM matchup_votes WHERE anon_id = $1 AND kind = 'skip'", anonID)
	if picks != 3 || skips != 5 {
		t.Errorf("picks=%d skips=%d, want 3/5", picks, skips)
	}
}

// TestSkipMatchup_AnonRejectsBracketMatchup — anon skip on a bracket
// child matchup is rejected with PermissionDenied (same rule as anon
// picks on bracket matchups).
func TestSkipMatchup_AnonRejectsBracketMatchup(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "alice", "alice@example.com", "TestPass123")
	bracket := seedTestBracket(t, db, owner.ID, "B", 4)

	if _, err := db.Exec("UPDATE brackets SET status = 'active' WHERE id = $1", bracket.ID); err != nil {
		t.Fatalf("activate bracket: %v", err)
	}
	m := seedTestMatchup(t, db, owner.ID, "bracket-child")
	if _, err := db.Exec(
		"UPDATE matchups SET bracket_id = $1, round = 1, status = 'active' WHERE id = $2",
		bracket.ID, m.ID,
	); err != nil {
		t.Fatalf("attach: %v", err)
	}

	h := &MatchupItemHandler{DB: db, ReadDB: db}
	anonID := skipAnonID

	_, err := h.SkipMatchup(context.Background(), connectReq(&matchupv1.SkipMatchupRequest{
		MatchupId: m.PublicID,
		AnonId:    &anonID,
	}, ""))
	assertConnectError(t, err, connect.CodePermissionDenied)

	// No row should land — bracket rejection happens before insert.
	var n int
	_ = db.Get(&n, "SELECT COUNT(*) FROM matchup_votes WHERE anon_id = $1", anonID)
	if n != 0 {
		t.Errorf("expected 0 rows after bracket rejection, got %d", n)
	}
}

// TestSkipMatchup_PickThenSkip_DecrementsItemVotes — converting a pick
// to a skip should decrement the previously-picked item's counter and
// rewrite the existing row (not insert a second).
func TestSkipMatchup_PickThenSkip_DecrementsItemVotes(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "alice", "alice@example.com", "TestPass123")
	m := seedTestMatchup(t, db, owner.ID, "switch-to-skip")

	itemID := firstItemPublicID(t, db, m.ID)
	h := &MatchupItemHandler{DB: db, ReadDB: db}
	anonID := skipAnonID

	// Pick first.
	if _, err := h.VoteItem(context.Background(), connectReq(&matchupv1.VoteItemRequest{
		Id:     itemID,
		AnonId: &anonID,
	}, "")); err != nil {
		t.Fatalf("seed pick: %v", err)
	}

	var votesBefore int
	_ = db.Get(&votesBefore, "SELECT votes FROM matchup_items WHERE public_id = $1", itemID)
	if votesBefore != 1 {
		t.Fatalf("expected item votes=1 after pick, got %d", votesBefore)
	}

	// Skip.
	if _, err := h.SkipMatchup(context.Background(), connectReq(&matchupv1.SkipMatchupRequest{
		MatchupId: m.PublicID,
		AnonId:    &anonID,
	}, "")); err != nil {
		t.Fatalf("skip after pick: %v", err)
	}

	// Item counter back to 0.
	var votesAfter int
	_ = db.Get(&votesAfter, "SELECT votes FROM matchup_items WHERE public_id = $1", itemID)
	if votesAfter != 0 {
		t.Errorf("expected item votes=0 after skip, got %d", votesAfter)
	}

	// Still exactly one row in matchup_votes — the pick was rewritten,
	// not duplicated.
	var n int
	_ = db.Get(&n, "SELECT COUNT(*) FROM matchup_votes WHERE anon_id = $1 AND matchup_public_id = $2", anonID, m.PublicID)
	if n != 1 {
		t.Errorf("expected 1 row after pick→skip, got %d", n)
	}

	// And it's now a skip.
	var kind string
	_ = db.Get(&kind, "SELECT kind FROM matchup_votes WHERE anon_id = $1 AND matchup_public_id = $2", anonID, m.PublicID)
	if kind != "skip" {
		t.Errorf("kind = %q after pick→skip, want %q", kind, "skip")
	}
}

// TestVoteItem_SkipThenPick_IncrementsItemVotes — converting a skip
// to a pick (via VoteItem) should rewrite the row to kind='pick',
// set the item reference, and increment the new item's counter.
func TestVoteItem_SkipThenPick_IncrementsItemVotes(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "alice", "alice@example.com", "TestPass123")
	m := seedTestMatchup(t, db, owner.ID, "skip-then-pick")

	itemID := firstItemPublicID(t, db, m.ID)
	h := &MatchupItemHandler{DB: db, ReadDB: db}
	anonID := skipAnonID

	// Skip first.
	if _, err := h.SkipMatchup(context.Background(), connectReq(&matchupv1.SkipMatchupRequest{
		MatchupId: m.PublicID,
		AnonId:    &anonID,
	}, "")); err != nil {
		t.Fatalf("seed skip: %v", err)
	}

	// Then pick.
	if _, err := h.VoteItem(context.Background(), connectReq(&matchupv1.VoteItemRequest{
		Id:     itemID,
		AnonId: &anonID,
	}, "")); err != nil {
		t.Fatalf("pick after skip: %v", err)
	}

	// Item votes should be 1 (skip didn't increment, pick did).
	var votes int
	_ = db.Get(&votes, "SELECT votes FROM matchup_items WHERE public_id = $1", itemID)
	if votes != 1 {
		t.Errorf("expected item votes=1 after skip→pick, got %d", votes)
	}

	// Row rewrite, not insert.
	var n int
	_ = db.Get(&n, "SELECT COUNT(*) FROM matchup_votes WHERE anon_id = $1 AND matchup_public_id = $2", anonID, m.PublicID)
	if n != 1 {
		t.Errorf("expected 1 row after skip→pick, got %d", n)
	}

	// Kind back to 'pick' with item set.
	var row struct {
		Kind                string  `db:"kind"`
		MatchupItemPublicID *string `db:"matchup_item_public_id"`
	}
	_ = db.Get(&row, "SELECT kind, matchup_item_public_id FROM matchup_votes WHERE anon_id = $1 AND matchup_public_id = $2", anonID, m.PublicID)
	if row.Kind != "pick" {
		t.Errorf("kind = %q, want %q", row.Kind, "pick")
	}
	if row.MatchupItemPublicID == nil || *row.MatchupItemPublicID != itemID {
		t.Errorf("item ref = %v, want %q", row.MatchupItemPublicID, itemID)
	}
}
