//go:build integration

package controllers

import (
	"context"
	"testing"

	matchupv1 "Matchup/gen/matchup/v1"

	"connectrpc.com/connect"
)

// Anonymous-vote feature tests.
//
// Cap is enforced server-side: 3 votes per anon UUID across all
// standalone matchups. Bracket child matchups can never be voted on
// anonymously regardless of count. Switching a vote between items
// on the SAME matchup doesn't burn a slot — only fresh votes on
// new matchups do.

const testAnonID = "test-anon-uuid-1234"

// helper: fetch the public_id of one item in a matchup.
func firstItemPublicID(t *testing.T, db interface {
	Get(dest interface{}, query string, args ...interface{}) error
}, matchupID uint) string {
	t.Helper()
	var pid string
	if err := db.Get(&pid,
		"SELECT public_id FROM matchup_items WHERE matchup_id = $1 ORDER BY id ASC LIMIT 1",
		matchupID,
	); err != nil {
		t.Fatalf("lookup item public_id: %v", err)
	}
	return pid
}

// TestVoteItem_AnonAllowsFirstThree — three sequential VoteItem calls
// against three different standalone matchups all succeed for the same
// anon UUID.
func TestVoteItem_AnonAllowsFirstThree(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "alice", "alice@example.com", "TestPass123")

	h := &MatchupItemHandler{DB: db, ReadDB: db}
	anonID := testAnonID

	for i := 0; i < 3; i++ {
		m := seedTestMatchup(t, db, owner.ID, "matchup")
		itemID := firstItemPublicID(t, db, m.ID)
		if _, err := h.VoteItem(context.Background(), connectReq(&matchupv1.VoteItemRequest{
			Id:     itemID,
			AnonId: &anonID,
		}, "")); err != nil {
			t.Fatalf("vote %d: %v", i+1, err)
		}
	}

	// Sanity: 3 rows landed in matchup_votes for this anon.
	var n int
	_ = db.Get(&n, "SELECT COUNT(*) FROM matchup_votes WHERE anon_id = $1", anonID)
	if n != 3 {
		t.Errorf("expected 3 anon votes, got %d", n)
	}
}

// TestVoteItem_AnonRejectsFourth — same anon UUID, fourth distinct
// matchup, server returns CodeResourceExhausted.
func TestVoteItem_AnonRejectsFourth(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "alice", "alice@example.com", "TestPass123")

	h := &MatchupItemHandler{DB: db, ReadDB: db}
	anonID := testAnonID

	// Burn the 3-vote allowance.
	for i := 0; i < 3; i++ {
		m := seedTestMatchup(t, db, owner.ID, "m")
		itemID := firstItemPublicID(t, db, m.ID)
		if _, err := h.VoteItem(context.Background(), connectReq(&matchupv1.VoteItemRequest{
			Id:     itemID,
			AnonId: &anonID,
		}, "")); err != nil {
			t.Fatalf("seed vote %d: %v", i+1, err)
		}
	}

	// Fourth fresh matchup → cap fires.
	m := seedTestMatchup(t, db, owner.ID, "fourth")
	itemID := firstItemPublicID(t, db, m.ID)
	_, err := h.VoteItem(context.Background(), connectReq(&matchupv1.VoteItemRequest{
		Id:     itemID,
		AnonId: &anonID,
	}, ""))
	assertConnectError(t, err, connect.CodeResourceExhausted)
}

// TestVoteItem_AnonSwitchVoteDoesNotCount — switching the vote between
// items on the same matchup doesn't burn a free slot. Critical UX
// detail: a user changing their mind on matchup A shouldn't lose
// future matchup capacity.
func TestVoteItem_AnonSwitchVoteDoesNotCount(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "alice", "alice@example.com", "TestPass123")
	m := seedTestMatchup(t, db, owner.ID, "switchable")

	var items []struct {
		PublicID string `db:"public_id"`
	}
	if err := db.Select(&items,
		"SELECT public_id FROM matchup_items WHERE matchup_id = $1 ORDER BY id ASC LIMIT 2",
		m.ID,
	); err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) < 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	h := &MatchupItemHandler{DB: db, ReadDB: db}
	anonID := testAnonID

	// First vote on item A.
	if _, err := h.VoteItem(context.Background(), connectReq(&matchupv1.VoteItemRequest{
		Id: items[0].PublicID, AnonId: &anonID,
	}, "")); err != nil {
		t.Fatalf("first vote: %v", err)
	}
	// Switch to item B.
	if _, err := h.VoteItem(context.Background(), connectReq(&matchupv1.VoteItemRequest{
		Id: items[1].PublicID, AnonId: &anonID,
	}, "")); err != nil {
		t.Fatalf("switch vote: %v", err)
	}

	// Still only one row → the switch updated, didn't insert.
	var n int
	_ = db.Get(&n, "SELECT COUNT(*) FROM matchup_votes WHERE anon_id = $1", anonID)
	if n != 1 {
		t.Errorf("expected 1 vote row after switch, got %d", n)
	}

	// Two more matchups should still succeed.
	for i := 0; i < 2; i++ {
		nm := seedTestMatchup(t, db, owner.ID, "next")
		nid := firstItemPublicID(t, db, nm.ID)
		if _, err := h.VoteItem(context.Background(), connectReq(&matchupv1.VoteItemRequest{
			Id: nid, AnonId: &anonID,
		}, "")); err != nil {
			t.Fatalf("post-switch vote %d: %v", i+1, err)
		}
	}
	// Third additional matchup → fourth row total → cap.
	nm := seedTestMatchup(t, db, owner.ID, "overflow")
	nid := firstItemPublicID(t, db, nm.ID)
	_, err := h.VoteItem(context.Background(), connectReq(&matchupv1.VoteItemRequest{
		Id: nid, AnonId: &anonID,
	}, ""))
	assertConnectError(t, err, connect.CodeResourceExhausted)
}

// TestVoteItem_AnonRejectsBracketMatchup — anonymous voters can never
// vote on a bracket-child matchup, even on their first attempt.
func TestVoteItem_AnonRejectsBracketMatchup(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "alice", "alice@example.com", "TestPass123")
	bracket := seedTestBracket(t, db, owner.ID, "B", 4)

	// Activate the bracket so the inner active-bracket gates aren't
	// the thing that fires first.
	if _, err := db.Exec("UPDATE brackets SET status = 'active' WHERE id = $1", bracket.ID); err != nil {
		t.Fatalf("activate bracket: %v", err)
	}

	// Attach a matchup to the bracket as a child.
	m := seedTestMatchup(t, db, owner.ID, "child")
	if _, err := db.Exec(
		"UPDATE matchups SET bracket_id = $1, round = 1, status = 'active' WHERE id = $2",
		bracket.ID, m.ID,
	); err != nil {
		t.Fatalf("attach matchup to bracket: %v", err)
	}

	itemID := firstItemPublicID(t, db, m.ID)
	anonID := testAnonID

	h := &MatchupItemHandler{DB: db, ReadDB: db}
	_, err := h.VoteItem(context.Background(), connectReq(&matchupv1.VoteItemRequest{
		Id: itemID, AnonId: &anonID,
	}, ""))
	assertConnectError(t, err, connect.CodePermissionDenied)

	// Verify NO row was inserted (cap-counter shouldn't be polluted).
	var n int
	_ = db.Get(&n, "SELECT COUNT(*) FROM matchup_votes WHERE anon_id = $1", anonID)
	if n != 0 {
		t.Errorf("expected 0 anon votes after bracket rejection, got %d", n)
	}
}

// TestVoteItem_AuthedUserNotCapped — sanity that the cap doesn't
// accidentally apply to signed-in users. VoteItem reads the auth
// token off the request's Authorization header (not the context),
// so we use connectReq with a real createTestToken value.
func TestVoteItem_AuthedUserNotCapped(t *testing.T) {
	db := setupTestDB(t)
	voter := seedTestUser(t, db, "voter", "v@example.com", "TestPass123")
	owner := seedTestUser(t, db, "owner", "o@example.com", "TestPass123")

	h := &MatchupItemHandler{DB: db, ReadDB: db}
	token := createTestToken(t, voter.ID)

	// Five votes across five matchups should all succeed.
	for i := 0; i < 5; i++ {
		m := seedTestMatchup(t, db, owner.ID, "m")
		itemID := firstItemPublicID(t, db, m.ID)
		if _, err := h.VoteItem(context.Background(), connectReq(&matchupv1.VoteItemRequest{
			Id: itemID,
		}, token)); err != nil {
			t.Fatalf("authed vote %d: %v", i+1, err)
		}
	}

	var n int
	_ = db.Get(&n, "SELECT COUNT(*) FROM matchup_votes WHERE user_id = $1", voter.ID)
	if n != 5 {
		t.Errorf("expected 5 authed votes, got %d", n)
	}
}

// TestGetAnonVoteStatus_ReturnsCount — happy path.
func TestGetAnonVoteStatus_ReturnsCount(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "alice", "alice@example.com", "TestPass123")

	h := &MatchupItemHandler{DB: db, ReadDB: db}
	anonID := testAnonID

	// Seed two anon votes via the handler so they go through the
	// real code path.
	for i := 0; i < 2; i++ {
		m := seedTestMatchup(t, db, owner.ID, "m")
		itemID := firstItemPublicID(t, db, m.ID)
		if _, err := h.VoteItem(context.Background(), connectReq(&matchupv1.VoteItemRequest{
			Id: itemID, AnonId: &anonID,
		}, "")); err != nil {
			t.Fatalf("seed vote: %v", err)
		}
	}

	resp, err := h.GetAnonVoteStatus(context.Background(), connectReq(&matchupv1.GetAnonVoteStatusRequest{
		AnonId: anonID,
	}, ""))
	if err != nil {
		t.Fatalf("GetAnonVoteStatus: %v", err)
	}
	if resp.Msg.GetUsed() != 2 {
		t.Errorf("expected used=2, got %d", resp.Msg.GetUsed())
	}
	if resp.Msg.GetMax() != int32(AnonVoteCap) {
		t.Errorf("expected max=%d, got %d", AnonVoteCap, resp.Msg.GetMax())
	}
}

// TestGetAnonVoteStatus_EmptyIDReturnsZero — the home page calls
// this on first load before localStorage has any anon UUID. Empty
// id must not error; it just reports used=0.
func TestGetAnonVoteStatus_EmptyIDReturnsZero(t *testing.T) {
	db := setupTestDB(t)

	h := &MatchupItemHandler{DB: db, ReadDB: db}
	resp, err := h.GetAnonVoteStatus(context.Background(), connectReq(&matchupv1.GetAnonVoteStatusRequest{
		AnonId: "",
	}, ""))
	if err != nil {
		t.Fatalf("GetAnonVoteStatus empty: %v", err)
	}
	if resp.Msg.GetUsed() != 0 || resp.Msg.GetMax() != int32(AnonVoteCap) {
		t.Errorf("expected used=0 max=%d, got used=%d max=%d",
			AnonVoteCap, resp.Msg.GetUsed(), resp.Msg.GetMax())
	}
}
