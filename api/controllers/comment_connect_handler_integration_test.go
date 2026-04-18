//go:build integration

package controllers

import (
	"context"
	"testing"

	commentv1 "Matchup/gen/comment/v1"

	"connectrpc.com/connect"
)

func TestCreateComment_Success(t *testing.T) {
	db := setupTestDB(t)
	user := seedTestUser(t, db, "commenter", "commenter@example.com", "TestPass123")
	matchup := seedTestMatchup(t, db, user.ID, "Comment Test Matchup")

	handler := &CommentHandler{DB: db, ReadDB: db}
	ctx := authedCtx(user.ID, false)

	resp, err := handler.CreateComment(ctx, connect.NewRequest(&commentv1.CreateCommentRequest{
		MatchupId: matchup.PublicID,
		Body:      "This is a test comment",
	}))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Msg.Comment == nil {
		t.Fatal("expected comment in response, got nil")
	}
	if resp.Msg.Comment.Body != "This is a test comment" {
		t.Errorf("expected body %q, got %q", "This is a test comment", resp.Msg.Comment.Body)
	}
}

func TestCreateComment_Unauthenticated(t *testing.T) {
	db := setupTestDB(t)
	user := seedTestUser(t, db, "commentauthor", "commentauthor@example.com", "TestPass123")
	matchup := seedTestMatchup(t, db, user.ID, "Unauth Comment Matchup")

	handler := &CommentHandler{DB: db, ReadDB: db}

	_, err := handler.CreateComment(context.Background(), connect.NewRequest(&commentv1.CreateCommentRequest{
		MatchupId: matchup.PublicID,
		Body:      "Should fail",
	}))
	assertConnectError(t, err, connect.CodeUnauthenticated)
}

func TestGetComments_Success(t *testing.T) {
	db := setupTestDB(t)
	user := seedTestUser(t, db, "getcommenter", "getcommenter@example.com", "TestPass123")
	matchup := seedTestMatchup(t, db, user.ID, "GetComments Matchup")

	seedTestComment(t, db, user.ID, matchup.ID, "First comment")
	seedTestComment(t, db, user.ID, matchup.ID, "Second comment")

	handler := &CommentHandler{DB: db, ReadDB: db}

	resp, err := handler.GetComments(context.Background(), connect.NewRequest(&commentv1.GetCommentsRequest{
		MatchupId: matchup.PublicID,
	}))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.Msg.Comments) < 2 {
		t.Errorf("expected at least 2 comments, got %d", len(resp.Msg.Comments))
	}
}

func TestDeleteComment_Success(t *testing.T) {
	db := setupTestDB(t)
	user := seedTestUser(t, db, "delcommenter", "delcommenter@example.com", "TestPass123")
	matchup := seedTestMatchup(t, db, user.ID, "DeleteComment Matchup")
	comment := seedTestComment(t, db, user.ID, matchup.ID, "To be deleted")

	handler := &CommentHandler{DB: db, ReadDB: db}
	ctx := authedCtx(user.ID, false)

	_, err := handler.DeleteComment(ctx, connect.NewRequest(&commentv1.DeleteCommentRequest{
		Id: comment.PublicID,
	}))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestDeleteComment_Unauthorized(t *testing.T) {
	db := setupTestDB(t)
	user1 := seedTestUser(t, db, "commentowner", "commentowner@example.com", "TestPass123")
	user2 := seedTestUser(t, db, "commentthief", "commentthief@example.com", "TestPass123")
	matchup := seedTestMatchup(t, db, user1.ID, "Unauthorized Delete Matchup")
	comment := seedTestComment(t, db, user1.ID, matchup.ID, "Not yours to delete")

	handler := &CommentHandler{DB: db, ReadDB: db}
	ctx := authedCtx(user2.ID, false)

	_, err := handler.DeleteComment(ctx, connect.NewRequest(&commentv1.DeleteCommentRequest{
		Id: comment.PublicID,
	}))
	assertConnectError(t, err, connect.CodePermissionDenied)
}
