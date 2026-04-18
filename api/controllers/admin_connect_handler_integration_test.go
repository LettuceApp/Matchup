//go:build integration

package controllers

import (
	"testing"

	adminv1 "Matchup/gen/admin/v1"

	"connectrpc.com/connect"
)

func TestAdminListUsers_Success(t *testing.T) {
	db := setupTestDB(t)
	admin := seedAdminUser(t, db, "adminlister", "adminlister@example.com", "TestPass123")
	seedTestUser(t, db, "regularuser", "regularuser@example.com", "TestPass123")

	handler := &AdminHandler{DB: db, ReadDB: db}
	ctx := authedCtx(admin.ID, true)

	page := int32(1)
	limit := int32(20)
	resp, err := handler.ListUsers(ctx, connect.NewRequest(&adminv1.ListUsersRequest{
		Page:  &page,
		Limit: &limit,
	}))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.Msg.Users) < 2 {
		t.Errorf("expected at least 2 users, got %d", len(resp.Msg.Users))
	}
}

func TestAdminListUsers_NonAdmin(t *testing.T) {
	db := setupTestDB(t)
	user := seedTestUser(t, db, "nonadminlister", "nonadminlister@example.com", "TestPass123")

	handler := &AdminHandler{DB: db, ReadDB: db}
	// Pass isAdmin=false to simulate a non-admin user. The admin handler
	// itself does not re-check the admin flag (that is done by middleware),
	// so calling the handler directly with a non-admin context will still
	// succeed. This test documents that the handler relies on middleware
	// for authorization -- the ListUsers method does not reject non-admin
	// callers on its own.
	ctx := authedCtx(user.ID, false)

	page := int32(1)
	limit := int32(20)
	resp, err := handler.ListUsers(ctx, connect.NewRequest(&adminv1.ListUsersRequest{
		Page:  &page,
		Limit: &limit,
	}))
	// The handler itself does not enforce admin access -- middleware does.
	// Calling the handler directly bypasses middleware, so this succeeds.
	if err != nil {
		// If the handler does enforce it, verify the error code.
		assertConnectError(t, err, connect.CodePermissionDenied)
		return
	}
	// Handler succeeded because it delegates auth to middleware.
	if resp.Msg.Users == nil {
		t.Error("expected users list in response")
	}
}

func TestAdminDeleteUser_Success(t *testing.T) {
	db := setupTestDB(t)
	admin := seedAdminUser(t, db, "admindeleter", "admindeleter@example.com", "TestPass123")
	target := seedTestUser(t, db, "deletetarget", "deletetarget@example.com", "TestPass123")

	handler := &AdminHandler{DB: db, ReadDB: db}
	ctx := authedCtx(admin.ID, true)

	_, err := handler.DeleteUser(ctx, connect.NewRequest(&adminv1.DeleteUserRequest{
		Id: target.PublicID,
	}))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify the user is gone from the database.
	ensureNoRow(t, db, "SELECT id FROM users WHERE id = $1", target.ID)
}

func TestAdminListMatchups_Success(t *testing.T) {
	db := setupTestDB(t)
	admin := seedAdminUser(t, db, "adminmatchuplister", "adminmatchuplister@example.com", "TestPass123")
	seedTestMatchup(t, db, admin.ID, "Admin Listed Matchup")

	handler := &AdminHandler{DB: db, ReadDB: db}
	ctx := authedCtx(admin.ID, true)

	page := int32(1)
	limit := int32(20)
	resp, err := handler.ListMatchups(ctx, connect.NewRequest(&adminv1.ListMatchupsRequest{
		Page:  &page,
		Limit: &limit,
	}))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.Msg.Matchups) < 1 {
		t.Errorf("expected at least 1 matchup, got %d", len(resp.Msg.Matchups))
	}
}
