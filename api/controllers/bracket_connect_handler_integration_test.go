//go:build integration

package controllers

import (
	"context"
	"testing"

	bracketv1 "Matchup/gen/bracket/v1"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
)

func TestCreateBracket_Success(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "owner", "owner@example.com", "TestPass123")
	handler := &BracketHandler{DB: db, ReadDB: db}

	desc := "test description"
	advance := "manual"
	resp, err := handler.CreateBracket(authedCtx(owner.ID, false), connectReq(&bracketv1.CreateBracketRequest{
		UserId:      owner.PublicID,
		Title:       "My First Bracket",
		Description: &desc,
		Size:        4,
		AdvanceMode: &advance,
		Entries:     []string{"A", "B", "C", "D"},
		Tags:        []string{"Music"},
	}, ""))
	if err != nil {
		t.Fatalf("CreateBracket: %v", err)
	}
	if resp.Msg.Bracket == nil {
		t.Fatal("expected bracket in response")
	}
	if resp.Msg.Bracket.Title != "My First Bracket" {
		t.Errorf("Title = %q, want %q", resp.Msg.Bracket.Title, "My First Bracket")
	}
	if resp.Msg.Bracket.Size != 4 {
		t.Errorf("Size = %d, want 4", resp.Msg.Bracket.Size)
	}
}

func TestCreateBracket_Unauthenticated(t *testing.T) {
	db := setupTestDB(t)
	handler := &BracketHandler{DB: db, ReadDB: db}

	_, err := handler.CreateBracket(context.Background(), connectReq(&bracketv1.CreateBracketRequest{
		UserId:  "some-public-id",
		Title:   "No Auth",
		Size:    4,
		Entries: []string{"A", "B", "C", "D"},
	}, ""))
	assertConnectError(t, err, connect.CodeUnauthenticated)
}

func TestGetBracket_Success(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "owner", "owner@example.com", "TestPass123")
	bracket := seedTestBracket(t, db, owner.ID, "Gettable", 4)
	handler := &BracketHandler{DB: db, ReadDB: db}

	resp, err := handler.GetBracket(authedCtx(owner.ID, false), connectReq(&bracketv1.GetBracketRequest{
		Id: bracket.PublicID,
	}, ""))
	if err != nil {
		t.Fatalf("GetBracket: %v", err)
	}
	if resp.Msg.Bracket == nil {
		t.Fatal("expected bracket")
	}
	if resp.Msg.Bracket.Title != "Gettable" {
		t.Errorf("Title = %q, want %q", resp.Msg.Bracket.Title, "Gettable")
	}
}

func TestGetBracket_NotFound(t *testing.T) {
	db := setupTestDB(t)
	handler := &BracketHandler{DB: db, ReadDB: db}

	_, err := handler.GetBracket(context.Background(), connectReq(&bracketv1.GetBracketRequest{
		Id: "nonexistent-public-id",
	}, ""))
	assertConnectError(t, err, connect.CodeNotFound)
}

func TestUpdateBracket_Success_OwnerOnly(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "owner", "owner@example.com", "TestPass123")
	bracket := seedTestBracket(t, db, owner.ID, "Updateable", 4)
	handler := &BracketHandler{DB: db, ReadDB: db}

	newTitle := "Updated Title"
	resp, err := handler.UpdateBracket(authedCtx(owner.ID, false), connectReq(&bracketv1.UpdateBracketRequest{
		Id:    bracket.PublicID,
		Title: &newTitle,
	}, ""))
	if err != nil {
		t.Fatalf("UpdateBracket: %v", err)
	}
	if resp.Msg.Bracket.Title != "Updated Title" {
		t.Errorf("Title = %q, want %q", resp.Msg.Bracket.Title, "Updated Title")
	}
}

func TestUpdateBracket_NotOwner_Forbidden(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "owner", "owner@example.com", "TestPass123")
	intruder := seedTestUser(t, db, "intruder", "intruder@example.com", "TestPass123")
	bracket := seedTestBracket(t, db, owner.ID, "Not Yours", 4)
	handler := &BracketHandler{DB: db, ReadDB: db}

	newTitle := "Hijacked"
	_, err := handler.UpdateBracket(authedCtx(intruder.ID, false), connectReq(&bracketv1.UpdateBracketRequest{
		Id:    bracket.PublicID,
		Title: &newTitle,
	}, ""))
	assertConnectError(t, err, connect.CodePermissionDenied)
}

func TestDeleteBracket_Success_OwnerOnly(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "owner", "owner@example.com", "TestPass123")
	bracket := seedTestBracket(t, db, owner.ID, "Deleteable", 4)
	handler := &BracketHandler{DB: db, ReadDB: db}

	_, err := handler.DeleteBracket(authedCtx(owner.ID, false), connectReq(&bracketv1.DeleteBracketRequest{
		Id: bracket.PublicID,
	}, ""))
	if err != nil {
		t.Fatalf("DeleteBracket: %v", err)
	}
	ensureNoRow(t, db, "SELECT id FROM brackets WHERE id = $1", bracket.ID)
}

func TestDeleteBracket_NotOwner_Forbidden(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "owner", "owner@example.com", "TestPass123")
	intruder := seedTestUser(t, db, "intruder", "intruder@example.com", "TestPass123")
	bracket := seedTestBracket(t, db, owner.ID, "Not Yours", 4)
	handler := &BracketHandler{DB: db, ReadDB: db}

	_, err := handler.DeleteBracket(authedCtx(intruder.ID, false), connectReq(&bracketv1.DeleteBracketRequest{
		Id: bracket.PublicID,
	}, ""))
	assertConnectError(t, err, connect.CodePermissionDenied)
}

// TestGetPopularBrackets_ExcludesNonActive is a regression test for the
// status filter: only brackets with status='active' should appear in the
// popular feed.
func TestGetPopularBrackets_ExcludesNonActive(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "owner", "owner@example.com", "TestPass123")

	draft := seedTestBracket(t, db, owner.ID, "Draft Bracket", 4)
	_ = draft // leave as draft (the default)

	active := seedTestBracket(t, db, owner.ID, "Active Bracket", 4)
	if _, err := db.Exec("UPDATE brackets SET status = 'active' WHERE id = $1", active.ID); err != nil {
		t.Fatalf("activate: %v", err)
	}

	refreshSnapshotsDirect(t, db)

	handler := &BracketHandler{DB: db, ReadDB: db}
	resp, err := handler.GetPopularBrackets(context.Background(),
		connectReq(&bracketv1.GetPopularBracketsRequest{}, ""))
	if err != nil {
		t.Fatalf("GetPopularBrackets: %v", err)
	}

	titles := map[string]bool{}
	for _, b := range resp.Msg.Brackets {
		titles[b.Title] = true
	}
	if titles["Draft Bracket"] {
		t.Error("draft bracket leaked into popular feed")
	}
	if !titles["Active Bracket"] {
		t.Error("expected 'Active Bracket' in popular feed")
	}
}

// TestGetPopularBrackets_IncludesAuthorUsernameAndCreatedAt confirms the
// PopularBracketData proto fields are populated (new as of this commit).
func TestGetPopularBrackets_IncludesAuthorUsernameAndCreatedAt(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "metadata_owner", "meta@example.com", "TestPass123")
	bracket := seedTestBracket(t, db, owner.ID, "Meta Bracket", 4)
	if _, err := db.Exec("UPDATE brackets SET status = 'active' WHERE id = $1", bracket.ID); err != nil {
		t.Fatalf("activate: %v", err)
	}

	refreshSnapshotsDirect(t, db)

	handler := &BracketHandler{DB: db, ReadDB: db}
	resp, err := handler.GetPopularBrackets(context.Background(),
		connectReq(&bracketv1.GetPopularBracketsRequest{}, ""))
	if err != nil {
		t.Fatalf("GetPopularBrackets: %v", err)
	}
	if len(resp.Msg.Brackets) == 0 {
		t.Fatal("expected at least one bracket")
	}
	var meta *bracketv1.PopularBracketData
	for _, b := range resp.Msg.Brackets {
		if b.Title == "Meta Bracket" {
			meta = b
			break
		}
	}
	if meta == nil {
		t.Fatal("'Meta Bracket' not in response")
	}
	if meta.AuthorUsername != "metadata_owner" {
		t.Errorf("AuthorUsername = %q, want %q", meta.AuthorUsername, "metadata_owner")
	}
	if meta.CreatedAt == "" {
		t.Error("CreatedAt should not be empty")
	}
}

func TestGetUserBrackets_ReturnsOwnerBrackets(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "listowner", "listowner@example.com", "TestPass123")
	for i := 0; i < 3; i++ {
		seedTestBracket(t, db, owner.ID, "B", 4)
	}
	handler := &BracketHandler{DB: db, ReadDB: db}

	resp, err := handler.GetUserBrackets(authedCtx(owner.ID, false), connectReq(&bracketv1.GetUserBracketsRequest{
		UserId: owner.PublicID,
		Page:   proto.Int32(1),
		Limit:  proto.Int32(10),
	}, ""))
	if err != nil {
		t.Fatalf("GetUserBrackets: %v", err)
	}
	if len(resp.Msg.Brackets) < 3 {
		t.Errorf("expected at least 3 brackets, got %d", len(resp.Msg.Brackets))
	}
}

func TestGetBracketSummary_IncludesBracketAndMatchups(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "summary_owner", "summary@example.com", "TestPass123")
	bracket := seedTestBracket(t, db, owner.ID, "Summary Bracket", 4)
	handler := &BracketHandler{DB: db, ReadDB: db}

	resp, err := handler.GetBracketSummary(authedCtx(owner.ID, false), connectReq(&bracketv1.GetBracketSummaryRequest{
		Id: bracket.PublicID,
	}, ""))
	if err != nil {
		t.Fatalf("GetBracketSummary: %v", err)
	}
	if resp.Msg.Summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if resp.Msg.Summary.Bracket == nil {
		t.Fatal("expected bracket in summary")
	}
	if resp.Msg.Summary.Bracket.Title != "Summary Bracket" {
		t.Errorf("Title = %q", resp.Msg.Summary.Bracket.Title)
	}
}
