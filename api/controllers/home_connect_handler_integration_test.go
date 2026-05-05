//go:build integration

package controllers

import (
	"context"
	"testing"
	"time"

	homev1 "Matchup/gen/home/v1"
	"Matchup/models"
)

func TestGetHomeSummary_EmptyDB(t *testing.T) {
	db := setupTestDB(t)
	handler := &HomeHandler{DB: db, ReadDB: db}

	resp, err := handler.GetHomeSummary(context.Background(),
		connectReq(&homev1.GetHomeSummaryRequest{}, ""))
	if err != nil {
		t.Fatalf("expected no error on empty DB, got %v", err)
	}
	if resp.Msg.Summary == nil {
		t.Fatal("expected non-nil summary even on empty DB")
	}
	s := resp.Msg.Summary
	if len(s.PopularMatchups) != 0 {
		t.Errorf("expected no popular matchups, got %d", len(s.PopularMatchups))
	}
	if len(s.PopularBrackets) != 0 {
		t.Errorf("expected no popular brackets, got %d", len(s.PopularBrackets))
	}
	if len(s.TrendingMatchups) != 0 {
		t.Errorf("expected no trending matchups, got %d", len(s.TrendingMatchups))
	}
	if len(s.MostPlayedMatchups) != 0 {
		t.Errorf("expected no most-played matchups, got %d", len(s.MostPlayedMatchups))
	}
	if s.VotesToday != 0 {
		t.Errorf("expected votes_today 0, got %d", s.VotesToday)
	}
}

func TestGetHomeSummary_PopularBrackets_IncludesNewActiveBracketWithoutEngagement(t *testing.T) {
	// Regression test for migration 013: a freshly-created active bracket
	// with zero engagement MUST appear on the homepage. Before 013, the MV
	// had `WHERE engagement_score > 0` which hid any new bracket until
	// someone liked, commented, or voted on it.
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "owner", "owner@example.com", "TestPass123")
	bracket := seedTestBracket(t, db, owner.ID, "Fresh Bracket", 4)

	// Seeded brackets are 'draft' by default — promote to active so the
	// status filter passes.
	if _, err := db.Exec("UPDATE brackets SET status = 'active' WHERE id = $1", bracket.ID); err != nil {
		t.Fatalf("activate bracket: %v", err)
	}

	refreshSnapshotsDirect(t, db)

	handler := &HomeHandler{DB: db, ReadDB: db}
	resp, err := handler.GetHomeSummary(context.Background(),
		connectReq(&homev1.GetHomeSummaryRequest{}, ""))
	if err != nil {
		t.Fatalf("GetHomeSummary: %v", err)
	}
	if len(resp.Msg.Summary.PopularBrackets) == 0 {
		t.Fatal("expected new active bracket in popular_brackets, got empty slice (migration 013 regression)")
	}
	found := false
	for _, b := range resp.Msg.Summary.PopularBrackets {
		if b.Title == "Fresh Bracket" {
			found = true
			if b.AuthorUsername != "owner" {
				t.Errorf("AuthorUsername = %q, want %q", b.AuthorUsername, "owner")
			}
			if b.CreatedAt == "" {
				t.Error("CreatedAt should be non-empty")
			}
			break
		}
	}
	if !found {
		t.Errorf("expected 'Fresh Bracket' in response, got %+v", resp.Msg.Summary.PopularBrackets)
	}
}

func TestGetHomeSummary_PopularBrackets_ExcludesNonActiveBrackets(t *testing.T) {
	// Drafts, completed, archived — none should appear on the homepage.
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "owner", "owner@example.com", "TestPass123")

	cases := []struct {
		title  string
		status string
	}{
		{"Draft Bracket", "draft"},
		{"Completed Bracket", "completed"},
		{"Archived Bracket", "archived"},
	}
	for _, c := range cases {
		b := seedTestBracket(t, db, owner.ID, c.title, 4)
		if _, err := db.Exec("UPDATE brackets SET status = $1 WHERE id = $2", c.status, b.ID); err != nil {
			t.Fatalf("set status %s: %v", c.status, err)
		}
	}

	// Also seed one active one to confirm it shows while the others don't.
	active := seedTestBracket(t, db, owner.ID, "Active Bracket", 4)
	if _, err := db.Exec("UPDATE brackets SET status = 'active' WHERE id = $1", active.ID); err != nil {
		t.Fatalf("activate bracket: %v", err)
	}

	refreshSnapshotsDirect(t, db)

	handler := &HomeHandler{DB: db, ReadDB: db}
	resp, err := handler.GetHomeSummary(context.Background(),
		connectReq(&homev1.GetHomeSummaryRequest{}, ""))
	if err != nil {
		t.Fatalf("GetHomeSummary: %v", err)
	}
	for _, b := range resp.Msg.Summary.PopularBrackets {
		switch b.Title {
		case "Draft Bracket", "Completed Bracket", "Archived Bracket":
			t.Errorf("non-active bracket %q leaked onto homepage", b.Title)
		}
	}
	// Active one should be there.
	foundActive := false
	for _, b := range resp.Msg.Summary.PopularBrackets {
		if b.Title == "Active Bracket" {
			foundActive = true
			break
		}
	}
	if !foundActive {
		t.Error("expected 'Active Bracket' in response")
	}
}

func TestGetHomeSummary_TopCreator_SkipsViewer(t *testing.T) {
	// When the viewer is the top creator, they shouldn't be shown as a
	// suggestion to follow.
	db := setupTestDB(t)
	viewer := seedTestUser(t, db, "viewer", "viewer@example.com", "TestPass123")
	// Viewer has no followers so is unlikely to be top anyway, but to be
	// thorough we also seed another user who will rank higher.
	_ = seedTestUser(t, db, "other", "other@example.com", "TestPass123")

	refreshSnapshotsDirect(t, db)

	handler := &HomeHandler{DB: db, ReadDB: db}
	resp, err := handler.GetHomeSummary(authedCtx(viewer.ID, false),
		connectReq(&homev1.GetHomeSummaryRequest{}, ""))
	if err != nil {
		t.Fatalf("GetHomeSummary: %v", err)
	}
	for _, c := range resp.Msg.Summary.CreatorsToFollow {
		if c.Username == "viewer" {
			t.Errorf("viewer should not appear in CreatorsToFollow, got %+v", c)
		}
	}
}

func TestGetHomeSummary_NewThisWeek_ExcludesNonActiveMatchups(t *testing.T) {
	// Regression test for migration 012: home_new_this_week_snapshot must
	// filter by status. Before 012 a draft/completed matchup created in the
	// past 7 days would appear in the "New This Week" section.
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "owner", "owner@example.com", "TestPass123")

	// Seed one active standalone matchup (should appear) and one completed
	// (should NOT appear).
	active := seedTestMatchup(t, db, owner.ID, "Active Standalone")
	if _, err := db.Exec("UPDATE matchups SET status = 'active' WHERE id = $1", active.ID); err != nil {
		t.Fatalf("activate active matchup: %v", err)
	}
	completed := seedTestMatchup(t, db, owner.ID, "Completed Standalone")
	if _, err := db.Exec("UPDATE matchups SET status = 'completed' WHERE id = $1", completed.ID); err != nil {
		t.Fatalf("complete matchup: %v", err)
	}

	refreshSnapshotsDirect(t, db)

	handler := &HomeHandler{DB: db, ReadDB: db}
	resp, err := handler.GetHomeSummary(context.Background(),
		connectReq(&homev1.GetHomeSummaryRequest{}, ""))
	if err != nil {
		t.Fatalf("GetHomeSummary: %v", err)
	}
	for _, m := range resp.Msg.Summary.NewThisWeek {
		if m.Title == "Completed Standalone" {
			t.Errorf("completed matchup leaked into NewThisWeek: %+v", m)
		}
	}
}

func TestGetHomeSummary_NewThisWeek_SkipsBracketChildren(t *testing.T) {
	// The MV is restricted to standalone matchups (bracket_id IS NULL).
	// A matchup attached to a bracket shouldn't appear in "New This Week".
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "owner", "owner@example.com", "TestPass123")
	bracket := seedTestBracket(t, db, owner.ID, "Parent Bracket", 4)

	child := seedTestMatchup(t, db, owner.ID, "Bracket Child Matchup")
	// Attach to bracket via direct SQL (ordinarily done by AttachMatchup).
	if _, err := db.Exec(
		"UPDATE matchups SET bracket_id = $1, status = 'published' WHERE id = $2",
		bracket.ID, child.ID,
	); err != nil {
		t.Fatalf("attach matchup: %v", err)
	}

	standalone := seedTestMatchup(t, db, owner.ID, "Standalone Matchup")
	if _, err := db.Exec("UPDATE matchups SET status = 'active' WHERE id = $1", standalone.ID); err != nil {
		t.Fatalf("activate standalone: %v", err)
	}

	refreshSnapshotsDirect(t, db)

	handler := &HomeHandler{DB: db, ReadDB: db}
	resp, err := handler.GetHomeSummary(context.Background(),
		connectReq(&homev1.GetHomeSummaryRequest{}, ""))
	if err != nil {
		t.Fatalf("GetHomeSummary: %v", err)
	}
	for _, m := range resp.Msg.Summary.NewThisWeek {
		if m.Title == "Bracket Child Matchup" {
			t.Error("bracket-child matchup leaked into NewThisWeek (should be standalone-only)")
		}
	}
}

func TestGetHomeSummary_PrivateUserContentHiddenFromStrangers(t *testing.T) {
	// A private user's bracket shouldn't show up to a stranger viewer.
	db := setupTestDB(t)
	privateOwner := seedTestUser(t, db, "secretowner", "secret@example.com", "TestPass123")
	if _, err := db.Exec("UPDATE users SET is_private = true WHERE id = $1", privateOwner.ID); err != nil {
		t.Fatalf("make user private: %v", err)
	}
	stranger := seedTestUser(t, db, "stranger", "stranger@example.com", "TestPass123")

	bracket := seedTestBracket(t, db, privateOwner.ID, "Secret Bracket", 4)
	if _, err := db.Exec("UPDATE brackets SET status = 'active' WHERE id = $1", bracket.ID); err != nil {
		t.Fatalf("activate bracket: %v", err)
	}

	refreshSnapshotsDirect(t, db)

	handler := &HomeHandler{DB: db, ReadDB: db}
	resp, err := handler.GetHomeSummary(authedCtx(stranger.ID, false),
		connectReq(&homev1.GetHomeSummaryRequest{}, ""))
	if err != nil {
		t.Fatalf("GetHomeSummary: %v", err)
	}
	for _, b := range resp.Msg.Summary.PopularBrackets {
		if b.Title == "Secret Bracket" {
			t.Error("private user's bracket leaked to stranger viewer")
		}
	}
}

func TestGetHomeSummary_AdminSeesPrivateContent(t *testing.T) {
	// Admins bypass the visibility check — they see everything.
	db := setupTestDB(t)
	privateOwner := seedTestUser(t, db, "secretowner", "secret@example.com", "TestPass123")
	if _, err := db.Exec("UPDATE users SET is_private = true WHERE id = $1", privateOwner.ID); err != nil {
		t.Fatalf("make user private: %v", err)
	}
	admin := seedAdminUser(t, db, "admin", "admin@example.com", "TestPass123")

	bracket := seedTestBracket(t, db, privateOwner.ID, "Secret Bracket Admin Visible", 4)
	if _, err := db.Exec("UPDATE brackets SET status = 'active' WHERE id = $1", bracket.ID); err != nil {
		t.Fatalf("activate bracket: %v", err)
	}

	refreshSnapshotsDirect(t, db)

	handler := &HomeHandler{DB: db, ReadDB: db}
	resp, err := handler.GetHomeSummary(authedCtx(admin.ID, true),
		connectReq(&homev1.GetHomeSummaryRequest{}, ""))
	if err != nil {
		t.Fatalf("GetHomeSummary: %v", err)
	}
	found := false
	for _, b := range resp.Msg.Summary.PopularBrackets {
		if b.Title == "Secret Bracket Admin Visible" {
			found = true
			break
		}
	}
	if !found {
		t.Error("admin should see private user's active bracket")
	}
}

func TestGetHomeSummary_RuntimeStatusChange_HidesBracket(t *testing.T) {
	// Even if the MV still has a bracket (because the refresh cadence hasn't
	// caught up), the handler's runtime `bracket.Status != "active"` guard
	// should hide it.
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "owner", "owner@example.com", "TestPass123")
	bracket := seedTestBracket(t, db, owner.ID, "Was Active", 4)
	if _, err := db.Exec("UPDATE brackets SET status = 'active' WHERE id = $1", bracket.ID); err != nil {
		t.Fatalf("activate bracket: %v", err)
	}

	// Populate the MV while the bracket is active.
	refreshSnapshotsDirect(t, db)

	// Now archive the bracket WITHOUT refreshing the MV.
	if _, err := db.Exec("UPDATE brackets SET status = 'archived' WHERE id = $1", bracket.ID); err != nil {
		t.Fatalf("archive bracket: %v", err)
	}

	handler := &HomeHandler{DB: db, ReadDB: db}
	resp, err := handler.GetHomeSummary(context.Background(),
		connectReq(&homev1.GetHomeSummaryRequest{}, ""))
	if err != nil {
		t.Fatalf("GetHomeSummary: %v", err)
	}
	for _, b := range resp.Msg.Summary.PopularBrackets {
		if b.Title == "Was Active" {
			t.Error("archived bracket still visible — runtime status guard missing")
		}
	}
}

func TestGetHomeSummary_BracketCreatedAtIsRFC3339(t *testing.T) {
	// The created_at field should be RFC3339-formatted so the frontend can
	// parse it with `new Date()`.
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "owner", "owner@example.com", "TestPass123")
	bracket := seedTestBracket(t, db, owner.ID, "Time Bracket", 4)
	if _, err := db.Exec("UPDATE brackets SET status = 'active' WHERE id = $1", bracket.ID); err != nil {
		t.Fatalf("activate bracket: %v", err)
	}

	refreshSnapshotsDirect(t, db)

	handler := &HomeHandler{DB: db, ReadDB: db}
	resp, err := handler.GetHomeSummary(context.Background(),
		connectReq(&homev1.GetHomeSummaryRequest{}, ""))
	if err != nil {
		t.Fatalf("GetHomeSummary: %v", err)
	}
	for _, b := range resp.Msg.Summary.PopularBrackets {
		if b.Title != "Time Bracket" {
			continue
		}
		if b.CreatedAt == "" {
			t.Fatal("CreatedAt empty")
		}
		if _, err := time.Parse(time.RFC3339, b.CreatedAt); err != nil {
			t.Errorf("CreatedAt %q not RFC3339: %v", b.CreatedAt, err)
		}
		return
	}
	t.Error("Time Bracket not found in response")
}

// TestGetHomeSummary_PopularMatchups_ExcludesOwnerEngagement is the
// regression test for migration 015. The popular_matchups_snapshot
// engagement_score must NOT credit the matchup owner's own like /
// comment / vote — otherwise an owner can trivially self-boost
// themselves into the popular feed. The raw display counts
// (likes_count etc.) are allowed to include owner interactions; only
// the ranking signal is scrubbed.
//
// Setup: two matchups seeded in isolation. Matchup A receives only a
// self-like from its own author (engagement contribution should be
// zero). Matchup B receives a like from a different user (engagement
// should be positive). After refreshing the MV, only B should appear
// in the popular feed.
func TestGetHomeSummary_PopularMatchups_ExcludesOwnerEngagement(t *testing.T) {
	db := setupTestDB(t)

	ownerA := seedTestUser(t, db, "ownerA", "ownerA@example.com", "TestPass123")
	ownerB := seedTestUser(t, db, "ownerB", "ownerB@example.com", "TestPass123")
	other := seedTestUser(t, db, "other", "other@example.com", "TestPass123")

	matchupA := seedTestMatchup(t, db, ownerA.ID, "Self-Liked Matchup")
	matchupB := seedTestMatchup(t, db, ownerB.ID, "Friend-Liked Matchup")

	// Matchup A: the owner self-likes — this must NOT boost ranking.
	if _, err := db.Exec(
		`INSERT INTO likes (user_id, matchup_id, created_at, updated_at)
		 VALUES ($1, $2, NOW(), NOW())`,
		ownerA.ID, matchupA.ID,
	); err != nil {
		t.Fatalf("seed owner self-like on A: %v", err)
	}

	// Matchup B: a different user likes — this should count.
	if _, err := db.Exec(
		`INSERT INTO likes (user_id, matchup_id, created_at, updated_at)
		 VALUES ($1, $2, NOW(), NOW())`,
		other.ID, matchupB.ID,
	); err != nil {
		t.Fatalf("seed non-owner like on B: %v", err)
	}

	refreshSnapshotsDirect(t, db)

	handler := &HomeHandler{DB: db, ReadDB: db}
	resp, err := handler.GetHomeSummary(context.Background(),
		connectReq(&homev1.GetHomeSummaryRequest{}, ""))
	if err != nil {
		t.Fatalf("GetHomeSummary: %v", err)
	}

	titles := map[string]bool{}
	for _, m := range resp.Msg.Summary.PopularMatchups {
		titles[m.Title] = true
	}
	if titles["Self-Liked Matchup"] {
		t.Error("matchup with only an owner self-like leaked into popular — engagement_score should exclude owner interactions (migration 015 regression)")
	}
	if !titles["Friend-Liked Matchup"] {
		t.Error("expected 'Friend-Liked Matchup' in popular feed — third-party like should count toward engagement")
	}
}

// TestGetHomeSummary_PopularBrackets_ExcludesOwnerEngagement is the
// bracket-side counterpart to the matchup test above. Bracket author's
// self-interactions (on the bracket itself or on any child matchup)
// must not contribute to the bracket's engagement_score. Since
// popular_brackets_snapshot intentionally has no engagement > 0
// filter (migration 013), the bracket will still appear in the MV —
// but its engagement_score must report zero so a bracket with real
// viewer engagement always ranks above a self-boosted one.
func TestGetHomeSummary_PopularBrackets_ExcludesOwnerEngagement(t *testing.T) {
	db := setupTestDB(t)

	ownerA := seedTestUser(t, db, "ownerA", "ownerA@example.com", "TestPass123")
	ownerB := seedTestUser(t, db, "ownerB", "ownerB@example.com", "TestPass123")
	fan := seedTestUser(t, db, "fan", "fan@example.com", "TestPass123")

	bracketA := seedTestBracket(t, db, ownerA.ID, "Self-Liked Bracket", 4)
	bracketB := seedTestBracket(t, db, ownerB.ID, "Fan-Liked Bracket", 4)

	for _, b := range []*models.Bracket{bracketA, bracketB} {
		if _, err := db.Exec("UPDATE brackets SET status = 'active' WHERE id = $1", b.ID); err != nil {
			t.Fatalf("activate bracket %d: %v", b.ID, err)
		}
	}

	// Bracket A: owner self-likes their own bracket. Must NOT rank.
	if _, err := db.Exec(
		`INSERT INTO bracket_likes (user_id, bracket_id, created_at, updated_at)
		 VALUES ($1, $2, NOW(), NOW())`,
		ownerA.ID, bracketA.ID,
	); err != nil {
		t.Fatalf("seed owner self-like on bracket A: %v", err)
	}

	// Bracket B: a fan likes — must rank above A.
	if _, err := db.Exec(
		`INSERT INTO bracket_likes (user_id, bracket_id, created_at, updated_at)
		 VALUES ($1, $2, NOW(), NOW())`,
		fan.ID, bracketB.ID,
	); err != nil {
		t.Fatalf("seed fan like on bracket B: %v", err)
	}

	refreshSnapshotsDirect(t, db)

	handler := &HomeHandler{DB: db, ReadDB: db}
	resp, err := handler.GetHomeSummary(context.Background(),
		connectReq(&homev1.GetHomeSummaryRequest{}, ""))
	if err != nil {
		t.Fatalf("GetHomeSummary: %v", err)
	}

	// Both brackets should appear (no engagement > 0 filter post-013),
	// but B must rank strictly above A.
	var aRank, bRank int32
	for _, b := range resp.Msg.Summary.PopularBrackets {
		switch b.Title {
		case "Self-Liked Bracket":
			aRank = b.Rank
		case "Fan-Liked Bracket":
			bRank = b.Rank
		}
	}
	if aRank == 0 || bRank == 0 {
		t.Fatalf("missing brackets in response (A rank=%d, B rank=%d)", aRank, bRank)
	}
	if bRank >= aRank {
		t.Errorf("expected fan-liked bracket (B rank=%d) to rank above self-liked bracket (A rank=%d) — owner self-likes shouldn't drive ranking", bRank, aRank)
	}
}
