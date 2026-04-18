//go:build integration

package controllers

import (
	"context"
	"fmt"
	"testing"

	matchupv1 "Matchup/gen/matchup/v1"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
)

func TestCreateMatchup_Success(t *testing.T) {
	db := setupTestDB(t)
	user := seedTestUser(t, db, "creator", "creator@example.com", "TestPass123")
	handler := &MatchupHandler{DB: db, ReadDB: db, S3Client: nil}

	endMode := "manual"
	content := "Test matchup content"
	resp, err := handler.CreateMatchup(authedCtx(user.ID, false), connectReq(&matchupv1.CreateMatchupRequest{
		Title:   "Best Language",
		Content: &content,
		Items: []*matchupv1.MatchupItemCreate{
			{Item: "A"},
			{Item: "B"},
		},
		EndMode: &endMode,
		Tags:    []string{},
	}, ""))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Msg.Matchup == nil {
		t.Fatal("expected matchup in response, got nil")
	}
	if resp.Msg.Matchup.Title != "Best Language" {
		t.Errorf("expected title %q, got %q", "Best Language", resp.Msg.Matchup.Title)
	}
	if len(resp.Msg.Matchup.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Msg.Matchup.Items))
	}
}

func TestCreateMatchup_Unauthenticated(t *testing.T) {
	db := setupTestDB(t)
	handler := &MatchupHandler{DB: db, ReadDB: db, S3Client: nil}

	endMode := "manual"
	_, err := handler.CreateMatchup(context.Background(), connectReq(&matchupv1.CreateMatchupRequest{
		Title: "No Auth",
		Items: []*matchupv1.MatchupItemCreate{
			{Item: "A"},
			{Item: "B"},
		},
		EndMode: &endMode,
		Tags:    []string{},
	}, ""))
	assertConnectError(t, err, connect.CodeUnauthenticated)
}

func TestGetMatchup_Success(t *testing.T) {
	db := setupTestDB(t)
	user := seedTestUser(t, db, "getter", "getter@example.com", "TestPass123")
	matchup := seedTestMatchup(t, db, user.ID, "Get Me")
	handler := &MatchupHandler{DB: db, ReadDB: db, S3Client: nil}

	resp, err := handler.GetMatchup(authedCtx(user.ID, false), connectReq(&matchupv1.GetMatchupRequest{
		Id: matchup.PublicID,
	}, ""))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Msg.Matchup == nil {
		t.Fatal("expected matchup in response, got nil")
	}
	if resp.Msg.Matchup.Title != "Get Me" {
		t.Errorf("expected title %q, got %q", "Get Me", resp.Msg.Matchup.Title)
	}
	if len(resp.Msg.Matchup.Items) < 2 {
		t.Errorf("expected at least 2 items, got %d", len(resp.Msg.Matchup.Items))
	}
}

func TestGetMatchup_NotFound(t *testing.T) {
	db := setupTestDB(t)
	handler := &MatchupHandler{DB: db, ReadDB: db, S3Client: nil}

	_, err := handler.GetMatchup(context.Background(), connectReq(&matchupv1.GetMatchupRequest{
		Id: "nonexistent-public-id",
	}, ""))
	assertConnectError(t, err, connect.CodeNotFound)
}

func TestListMatchups_Success(t *testing.T) {
	db := setupTestDB(t)
	user := seedTestUser(t, db, "lister", "lister@example.com", "TestPass123")
	for i := 0; i < 3; i++ {
		seedTestMatchup(t, db, user.ID, fmt.Sprintf("Matchup %d", i))
	}
	handler := &MatchupHandler{DB: db, ReadDB: db, S3Client: nil}

	resp, err := handler.ListMatchups(authedCtx(user.ID, false), connectReq(&matchupv1.ListMatchupsRequest{
		Page:  proto.Int32(1),
		Limit: proto.Int32(10),
	}, ""))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.Msg.Matchups) < 3 {
		t.Errorf("expected at least 3 matchups, got %d", len(resp.Msg.Matchups))
	}
	if resp.Msg.Pagination == nil {
		t.Fatal("expected pagination in response, got nil")
	}
	if resp.Msg.Pagination.Total < 3 {
		t.Errorf("expected total >= 3, got %d", resp.Msg.Pagination.Total)
	}
}

func TestListMatchups_Pagination(t *testing.T) {
	db := setupTestDB(t)
	user := seedTestUser(t, db, "pager", "pager@example.com", "TestPass123")
	for i := 0; i < 3; i++ {
		seedTestMatchup(t, db, user.ID, fmt.Sprintf("Paged Matchup %d", i))
	}
	handler := &MatchupHandler{DB: db, ReadDB: db, S3Client: nil}

	resp, err := handler.ListMatchups(authedCtx(user.ID, false), connectReq(&matchupv1.ListMatchupsRequest{
		Page:  proto.Int32(1),
		Limit: proto.Int32(2),
	}, ""))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.Msg.Matchups) != 2 {
		t.Errorf("expected 2 matchups, got %d", len(resp.Msg.Matchups))
	}
	if resp.Msg.Pagination == nil {
		t.Fatal("expected pagination in response, got nil")
	}
	if resp.Msg.Pagination.Total < 3 {
		t.Errorf("expected total >= 3, got %d", resp.Msg.Pagination.Total)
	}
}

func TestDeleteMatchup_Success(t *testing.T) {
	db := setupTestDB(t)
	user := seedTestUser(t, db, "deleter", "deleter@example.com", "TestPass123")
	matchup := seedTestMatchup(t, db, user.ID, "Delete Me")
	handler := &MatchupHandler{DB: db, ReadDB: db, S3Client: nil}

	_, err := handler.DeleteMatchup(authedCtx(user.ID, false), connectReq(&matchupv1.DeleteMatchupRequest{
		Id: matchup.PublicID,
	}, ""))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	ensureNoRow(t, db, "SELECT id FROM matchups WHERE id = $1", matchup.ID)
}

func TestDeleteMatchup_Unauthorized(t *testing.T) {
	db := setupTestDB(t)
	user1 := seedTestUser(t, db, "owner", "owner@example.com", "TestPass123")
	user2 := seedTestUser(t, db, "intruder", "intruder@example.com", "TestPass123")
	matchup := seedTestMatchup(t, db, user1.ID, "Not Yours")
	handler := &MatchupHandler{DB: db, ReadDB: db, S3Client: nil}

	_, err := handler.DeleteMatchup(authedCtx(user2.ID, false), connectReq(&matchupv1.DeleteMatchupRequest{
		Id: matchup.PublicID,
	}, ""))
	assertConnectError(t, err, connect.CodePermissionDenied)
}

// TestListMatchups_ExcludesNonActiveStatuses is a regression test for the
// status filter fix. Before the fix, ListMatchups returned all matchups
// regardless of status, so drafts and completed matchups leaked onto the
// homepage "Latest" tab. After the fix, only active/published are returned.
func TestListMatchups_ExcludesNonActiveStatuses(t *testing.T) {
	db := setupTestDB(t)
	user := seedTestUser(t, db, "mixer", "mixer@example.com", "TestPass123")

	// seedTestMatchup defaults to "published" — that's one we expect visible.
	published := seedTestMatchup(t, db, user.ID, "Published One")

	active := seedTestMatchup(t, db, user.ID, "Active One")
	if _, err := db.Exec("UPDATE matchups SET status = 'active' WHERE id = $1", active.ID); err != nil {
		t.Fatalf("activate: %v", err)
	}

	draft := seedTestMatchup(t, db, user.ID, "Draft One")
	if _, err := db.Exec("UPDATE matchups SET status = 'draft' WHERE id = $1", draft.ID); err != nil {
		t.Fatalf("draft: %v", err)
	}

	completed := seedTestMatchup(t, db, user.ID, "Completed One")
	if _, err := db.Exec("UPDATE matchups SET status = 'completed' WHERE id = $1", completed.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}

	archived := seedTestMatchup(t, db, user.ID, "Archived One")
	if _, err := db.Exec("UPDATE matchups SET status = 'archived' WHERE id = $1", archived.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}

	handler := &MatchupHandler{DB: db, ReadDB: db, S3Client: nil}
	resp, err := handler.ListMatchups(authedCtx(user.ID, false), connectReq(&matchupv1.ListMatchupsRequest{
		Page:  proto.Int32(1),
		Limit: proto.Int32(50),
	}, ""))
	if err != nil {
		t.Fatalf("ListMatchups: %v", err)
	}

	titles := map[string]bool{}
	for _, m := range resp.Msg.Matchups {
		titles[m.Title] = true
	}
	if !titles["Published One"] {
		t.Error("expected 'Published One' (status=published) in results")
	}
	if !titles["Active One"] {
		t.Error("expected 'Active One' (status=active) in results")
	}
	if titles["Draft One"] {
		t.Error("draft matchup leaked into ListMatchups")
	}
	if titles["Completed One"] {
		t.Error("completed matchup leaked into ListMatchups")
	}
	if titles["Archived One"] {
		t.Error("archived matchup leaked into ListMatchups")
	}

	_ = published // suppress unused; we use its Title above
}

// TestListMatchups_PaginationTotalExcludesNonActive confirms the COUNT(*)
// query also respects the status filter. A mismatched total breaks the
// client-side pagination UI.
func TestListMatchups_PaginationTotalExcludesNonActive(t *testing.T) {
	db := setupTestDB(t)
	user := seedTestUser(t, db, "pager", "pager@example.com", "TestPass123")

	// 2 active, 3 drafts — total should report 2, not 5.
	for i := 0; i < 2; i++ {
		m := seedTestMatchup(t, db, user.ID, fmt.Sprintf("active-%d", i))
		if _, err := db.Exec("UPDATE matchups SET status = 'active' WHERE id = $1", m.ID); err != nil {
			t.Fatalf("activate: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		m := seedTestMatchup(t, db, user.ID, fmt.Sprintf("draft-%d", i))
		if _, err := db.Exec("UPDATE matchups SET status = 'draft' WHERE id = $1", m.ID); err != nil {
			t.Fatalf("draft: %v", err)
		}
	}

	handler := &MatchupHandler{DB: db, ReadDB: db, S3Client: nil}
	resp, err := handler.ListMatchups(authedCtx(user.ID, false), connectReq(&matchupv1.ListMatchupsRequest{
		Page:  proto.Int32(1),
		Limit: proto.Int32(10),
	}, ""))
	if err != nil {
		t.Fatalf("ListMatchups: %v", err)
	}
	if resp.Msg.Pagination == nil {
		t.Fatal("expected pagination")
	}
	if resp.Msg.Pagination.Total != 2 {
		t.Errorf("expected total=2 (only active), got %d", resp.Msg.Pagination.Total)
	}
}
