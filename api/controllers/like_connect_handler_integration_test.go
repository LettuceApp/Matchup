//go:build integration

package controllers

import (
	"context"
	"testing"

	likev1 "Matchup/gen/like/v1"

	"connectrpc.com/connect"
)

func TestLikeMatchup_Success(t *testing.T) {
	db := setupTestDB(t)
	author := seedTestUser(t, db, "likeauthor", "likeauthor@example.com", "TestPass123")
	matchup := seedTestMatchup(t, db, author.ID, "Like Matchup")
	liker := seedTestUser(t, db, "liker", "liker@example.com", "TestPass123")

	handler := &LikeHandler{DB: db, ReadDB: db}
	ctx := authedCtx(liker.ID, false)

	_, err := handler.LikeMatchup(ctx, connect.NewRequest(&likev1.LikeMatchupRequest{
		MatchupId: matchup.PublicID,
	}))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestLikeMatchup_Unauthenticated(t *testing.T) {
	db := setupTestDB(t)
	author := seedTestUser(t, db, "likeauthor2", "likeauthor2@example.com", "TestPass123")
	matchup := seedTestMatchup(t, db, author.ID, "Unauth Like Matchup")

	handler := &LikeHandler{DB: db, ReadDB: db}

	_, err := handler.LikeMatchup(context.Background(), connect.NewRequest(&likev1.LikeMatchupRequest{
		MatchupId: matchup.PublicID,
	}))
	assertConnectError(t, err, connect.CodeUnauthenticated)
}

func TestUnlikeMatchup_Success(t *testing.T) {
	db := setupTestDB(t)
	author := seedTestUser(t, db, "unlikeauthor", "unlikeauthor@example.com", "TestPass123")
	matchup := seedTestMatchup(t, db, author.ID, "Unlike Matchup")
	liker := seedTestUser(t, db, "unliker", "unliker@example.com", "TestPass123")

	handler := &LikeHandler{DB: db, ReadDB: db}
	ctx := authedCtx(liker.ID, false)

	// Like first
	_, err := handler.LikeMatchup(ctx, connect.NewRequest(&likev1.LikeMatchupRequest{
		MatchupId: matchup.PublicID,
	}))
	if err != nil {
		t.Fatalf("like setup failed: %v", err)
	}

	// Then unlike
	_, err = handler.UnlikeMatchup(ctx, connect.NewRequest(&likev1.UnlikeMatchupRequest{
		MatchupId: matchup.PublicID,
	}))
	if err != nil {
		t.Fatalf("expected no error on unlike, got %v", err)
	}
}

func TestGetLikes_Success(t *testing.T) {
	db := setupTestDB(t)
	author := seedTestUser(t, db, "getlikesauthor", "getlikesauthor@example.com", "TestPass123")
	matchup := seedTestMatchup(t, db, author.ID, "GetLikes Matchup")
	liker := seedTestUser(t, db, "getlikesliker", "getlikesliker@example.com", "TestPass123")

	handler := &LikeHandler{DB: db, ReadDB: db}

	// Seed a like
	ctx := authedCtx(liker.ID, false)
	_, err := handler.LikeMatchup(ctx, connect.NewRequest(&likev1.LikeMatchupRequest{
		MatchupId: matchup.PublicID,
	}))
	if err != nil {
		t.Fatalf("like setup failed: %v", err)
	}

	// Get likes (no auth required)
	resp, err := handler.GetLikes(context.Background(), connect.NewRequest(&likev1.GetLikesRequest{
		MatchupId: matchup.PublicID,
	}))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.Msg.Likes) < 1 {
		t.Errorf("expected at least 1 like, got %d", len(resp.Msg.Likes))
	}
}
