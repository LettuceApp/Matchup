//go:build integration

package controllers

import (
	"context"
	"fmt"
	"testing"

	userv1 "Matchup/gen/user/v1"

	"connectrpc.com/connect"
)

func TestCreateUser_Success(t *testing.T) {
	db := setupTestDB(t)
	handler := &UserHandler{DB: db, ReadDB: db, S3Client: nil}

	resp, err := handler.CreateUser(context.Background(), connectReq(&userv1.CreateUserRequest{
		Username: "newuser",
		Email:    "newuser@example.com",
		Password: "TestPass123",
	}, ""))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Msg.User.Id == "" {
		t.Error("expected non-empty user ID")
	}
	if resp.Msg.User.Username != "newuser" {
		t.Errorf("expected username %q, got %q", "newuser", resp.Msg.User.Username)
	}
	if resp.Msg.User.Email != "newuser@example.com" {
		t.Errorf("expected email %q, got %q", "newuser@example.com", resp.Msg.User.Email)
	}
}

func TestCreateUser_DuplicateEmail(t *testing.T) {
	db := setupTestDB(t)
	seedTestUser(t, db, "existing", "dupe@example.com", "TestPass123")
	handler := &UserHandler{DB: db, ReadDB: db, S3Client: nil}

	_, err := handler.CreateUser(context.Background(), connectReq(&userv1.CreateUserRequest{
		Username: "different",
		Email:    "dupe@example.com",
		Password: "TestPass123",
	}, ""))
	assertConnectError(t, err, connect.CodeAlreadyExists)
}

func TestCreateUser_DuplicateUsername(t *testing.T) {
	db := setupTestDB(t)
	seedTestUser(t, db, "taken", "original@example.com", "TestPass123")
	handler := &UserHandler{DB: db, ReadDB: db, S3Client: nil}

	_, err := handler.CreateUser(context.Background(), connectReq(&userv1.CreateUserRequest{
		Username: "taken",
		Email:    "different@example.com",
		Password: "TestPass123",
	}, ""))
	assertConnectError(t, err, connect.CodeAlreadyExists)
}

func TestCreateUser_MissingPassword(t *testing.T) {
	db := setupTestDB(t)
	handler := &UserHandler{DB: db, ReadDB: db, S3Client: nil}

	_, err := handler.CreateUser(context.Background(), connectReq(&userv1.CreateUserRequest{
		Username: "nopass",
		Email:    "nopass@example.com",
		Password: "",
	}, ""))
	assertConnectError(t, err, connect.CodeInvalidArgument)
}

func TestGetUser_Success(t *testing.T) {
	db := setupTestDB(t)
	user := seedTestUser(t, db, "getme", "getme@example.com", "TestPass123")
	handler := &UserHandler{DB: db, ReadDB: db, S3Client: nil}

	resp, err := handler.GetUser(context.Background(), connectReq(&userv1.GetUserRequest{
		Id: user.PublicID,
	}, ""))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Msg.User.Username != "getme" {
		t.Errorf("expected username %q, got %q", "getme", resp.Msg.User.Username)
	}
}

func TestGetUser_ByUsername(t *testing.T) {
	db := setupTestDB(t)
	seedTestUser(t, db, "findbyname", "findbyname@example.com", "TestPass123")
	handler := &UserHandler{DB: db, ReadDB: db, S3Client: nil}

	resp, err := handler.GetUser(context.Background(), connectReq(&userv1.GetUserRequest{
		Id: "findbyname",
	}, ""))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Msg.User.Username != "findbyname" {
		t.Errorf("expected username %q, got %q", "findbyname", resp.Msg.User.Username)
	}
}

func TestGetUser_NotFound(t *testing.T) {
	db := setupTestDB(t)
	handler := &UserHandler{DB: db, ReadDB: db, S3Client: nil}

	_, err := handler.GetUser(context.Background(), connectReq(&userv1.GetUserRequest{
		Id: "nonexistent-id-12345",
	}, ""))
	assertConnectError(t, err, connect.CodeNotFound)
}

func TestGetCurrentUser_Success(t *testing.T) {
	db := setupTestDB(t)
	user := seedTestUser(t, db, "currentuser", "current@example.com", "TestPass123")
	handler := &UserHandler{DB: db, ReadDB: db, S3Client: nil}

	ctx := authedCtx(user.ID, false)
	resp, err := handler.GetCurrentUser(ctx, connectReq(&userv1.GetCurrentUserRequest{}, ""))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Msg.User.Username != "currentuser" {
		t.Errorf("expected username %q, got %q", "currentuser", resp.Msg.User.Username)
	}
}

func TestGetCurrentUser_Unauthenticated(t *testing.T) {
	db := setupTestDB(t)
	handler := &UserHandler{DB: db, ReadDB: db, S3Client: nil}

	_, err := handler.GetCurrentUser(context.Background(), connectReq(&userv1.GetCurrentUserRequest{}, ""))
	assertConnectError(t, err, connect.CodeUnauthenticated)
}

func TestListUsers_Success(t *testing.T) {
	db := setupTestDB(t)
	seedTestUser(t, db, "listuser1", "list1@example.com", "TestPass123")
	seedTestUser(t, db, "listuser2", "list2@example.com", "TestPass123")
	handler := &UserHandler{DB: db, ReadDB: db, S3Client: nil}

	resp, err := handler.ListUsers(context.Background(), connectReq(&userv1.ListUsersRequest{}, ""))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.Msg.Users) < 2 {
		t.Errorf("expected at least 2 users, got %d", len(resp.Msg.Users))
	}
}

func TestFollowUser_Success(t *testing.T) {
	db := setupTestDB(t)
	alice := seedTestUser(t, db, "alice", "alice@example.com", "TestPass123")
	bob := seedTestUser(t, db, "bob", "bob@example.com", "TestPass123")
	handler := &UserHandler{DB: db, ReadDB: db, S3Client: nil}

	ctx := authedCtx(alice.ID, false)
	_, err := handler.FollowUser(ctx, connectReq(&userv1.FollowUserRequest{
		Id: bob.PublicID,
	}, ""))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify the follow row exists in the database.
	var count int
	err = db.Get(&count, "SELECT COUNT(*) FROM follows WHERE follower_id=$1 AND followed_id=$2", alice.ID, bob.ID)
	if err != nil {
		t.Fatalf("query follows: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 follow row, got %d", count)
	}
}

func TestUnfollowUser_Success(t *testing.T) {
	db := setupTestDB(t)
	alice := seedTestUser(t, db, "alice2", "alice2@example.com", "TestPass123")
	bob := seedTestUser(t, db, "bob2", "bob2@example.com", "TestPass123")

	// Insert a follow row directly so we can test unfollowing.
	_, err := db.Exec(
		"INSERT INTO follows (follower_id, followed_id, created_at) VALUES ($1, $2, NOW())",
		alice.ID, bob.ID,
	)
	if err != nil {
		t.Fatalf("insert follow row: %v", err)
	}
	// Update counters to match.
	db.Exec("UPDATE users SET following_count = 1 WHERE id = $1", alice.ID)
	db.Exec("UPDATE users SET followers_count = 1 WHERE id = $1", bob.ID)

	handler := &UserHandler{DB: db, ReadDB: db, S3Client: nil}

	ctx := authedCtx(alice.ID, false)
	_, err = handler.UnfollowUser(ctx, connectReq(&userv1.UnfollowUserRequest{
		Id: bob.PublicID,
	}, ""))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify the follow row no longer exists.
	var count int
	err = db.Get(&count, fmt.Sprintf("SELECT COUNT(*) FROM follows WHERE follower_id=$1 AND followed_id=$2"), alice.ID, bob.ID)
	if err != nil {
		t.Fatalf("query follows: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 follow rows, got %d", count)
	}
}
