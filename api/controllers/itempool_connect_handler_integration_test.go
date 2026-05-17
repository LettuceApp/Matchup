//go:build integration

package controllers

import (
	"testing"

	itempoolv1 "Matchup/gen/itempool/v1"
)

func TestItemPoolHandler_CreateAndGetItemPool(t *testing.T) {
	db := setupTestDB(t)
	user := seedTestUser(t, db, "poolowner", "owner@example.com", "TestPass123")
	
	// Ensure email is verified so create doesn't fail
	if _, err := db.Exec("UPDATE users SET email_verified = true WHERE id = $1", user.ID); err != nil {
		t.Fatalf("verify email: %v", err)
	}

	handler := &ItemPoolHandler{DB: db, ReadDB: db}

	// 1. Create Item Pool
	createResp, err := handler.CreateItemPool(authedCtx(user.ID, false), connectReq(&itempoolv1.CreateItemPoolRequest{
		Title: "My Favorite Fighters",
		Items: []*itempoolv1.ItemPoolItemCreate{
			{Item: "Ryu"},
			{Item: "Ken"},
			{Item: "Chun-Li"},
		},
	}, ""))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	ip := createResp.Msg.ItemPool
	if ip.Title != "My Favorite Fighters" {
		t.Errorf("expected title My Favorite Fighters, got %v", ip.Title)
	}
	if ip.Size != 3 {
		t.Errorf("expected size 3, got %v", ip.Size)
	}

	// 2. Get Item Pool
	getResp, err := handler.GetItemPool(authedCtx(user.ID, false), connectReq(&itempoolv1.GetItemPoolRequest{
		Id: ip.Id,
	}, ""))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if getResp.Msg.ItemPool.Title != "My Favorite Fighters" {
		t.Errorf("expected title My Favorite Fighters, got %v", getResp.Msg.ItemPool.Title)
	}

	// 3. Update Item Pool
	titleUpdate := "Updated Fighters"
	updateResp, err := handler.UpdateItemPool(authedCtx(user.ID, false), connectReq(&itempoolv1.UpdateItemPoolRequest{
		Id:    ip.Id,
		Title: &titleUpdate,
		Items: []*itempoolv1.ItemPoolItemCreate{
			{Item: "Ryu"},
			{Item: "Ken"},
			{Item: "Chun-Li"},
			{Item: "Guile"},
		},
	}, ""))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if updateResp.Msg.ItemPool.Title != "Updated Fighters" {
		t.Errorf("expected title Updated Fighters, got %v", updateResp.Msg.ItemPool.Title)
	}
	if updateResp.Msg.ItemPool.Size != 4 {
		t.Errorf("expected size 4, got %v", updateResp.Msg.ItemPool.Size)
	}

	// 4. Delete Item Pool
	_, err = handler.DeleteItemPool(authedCtx(user.ID, false), connectReq(&itempoolv1.DeleteItemPoolRequest{
		Id: ip.Id,
	}, ""))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// 5. Verify Deletion
	_, err = handler.GetItemPool(authedCtx(user.ID, false), connectReq(&itempoolv1.GetItemPoolRequest{
		Id: ip.Id,
	}, ""))
	if err == nil {
		t.Fatalf("expected error on get after delete, got nil")
	}
}
