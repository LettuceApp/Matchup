//go:build integration

package controllers

import (
	"context"
	"testing"

	activityv1 "Matchup/gen/activity/v1"
	httpctx "Matchup/utils/httpctx"
)

// Helper — web platform is the common case; keeps each test's intent
// front-and-centre.
func webSubscribeReq(endpoint, p256, auth string) *activityv1.SubscribePushRequest {
	p := p256
	a := auth
	return &activityv1.SubscribePushRequest{
		Platform:  "web",
		Endpoint:  endpoint,
		P256DhKey: &p,
		AuthKey:   &a,
	}
}

// TestSubscribePush_WebUpsertsRow — first call inserts; second call
// with the same endpoint updates the existing row (ON CONFLICT DO
// UPDATE). New key material overwrites old — handles the "browser
// generated a fresh key pair" case.
func TestSubscribePush_WebUpsertsRow(t *testing.T) {
	db := setupTestDB(t)
	me := seedTestUser(t, db, "me", "me@example.com", "TestPass123")
	ctx := httpctx.WithUserID(context.Background(), me.ID)
	h := &ActivityHandler{DB: db, ReadDB: db}

	resp1, err := h.SubscribePush(ctx, connectReq(
		webSubscribeReq("https://fcm.example/abc", "p-key-v1", "auth-v1"), ""))
	if err != nil {
		t.Fatalf("first subscribe: %v", err)
	}
	if resp1.Msg.Id == 0 {
		t.Fatal("expected non-zero id on first subscribe")
	}

	// Second call with same endpoint but new key material.
	resp2, err := h.SubscribePush(ctx, connectReq(
		webSubscribeReq("https://fcm.example/abc", "p-key-v2", "auth-v2"), ""))
	if err != nil {
		t.Fatalf("second subscribe: %v", err)
	}
	if resp2.Msg.Id != resp1.Msg.Id {
		t.Errorf("expected upsert to keep id %d, got %d", resp1.Msg.Id, resp2.Msg.Id)
	}

	// Verify keys updated + platform stays 'web'.
	var p256, authKey, platform string
	if err := db.QueryRow(
		"SELECT p256dh_key, auth_key, platform FROM push_subscriptions WHERE endpoint=$1",
		"https://fcm.example/abc",
	).Scan(&p256, &authKey, &platform); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if p256 != "p-key-v2" || authKey != "auth-v2" {
		t.Errorf("expected upsert to refresh keys, got p256=%q auth=%q", p256, authKey)
	}
	if platform != "web" {
		t.Errorf("expected platform=web, got %q", platform)
	}
}

// TestSubscribePush_IOSStoresTokenWithoutWebKeys — native subscribe
// flow: only endpoint (APNS device token) is set; p256dh + auth stay
// NULL at the DB layer.
func TestSubscribePush_IOSStoresTokenWithoutWebKeys(t *testing.T) {
	db := setupTestDB(t)
	me := seedTestUser(t, db, "me", "me@example.com", "TestPass123")
	ctx := httpctx.WithUserID(context.Background(), me.ID)
	h := &ActivityHandler{DB: db, ReadDB: db}

	_, err := h.SubscribePush(ctx, connectReq(&activityv1.SubscribePushRequest{
		Platform: "ios",
		Endpoint: "apns-device-token-abc123",
	}, ""))
	if err != nil {
		t.Fatalf("ios subscribe: %v", err)
	}

	var platform string
	var p256, authKey *string
	if err := db.QueryRow(
		"SELECT platform, p256dh_key, auth_key FROM push_subscriptions WHERE endpoint=$1",
		"apns-device-token-abc123",
	).Scan(&platform, &p256, &authKey); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if platform != "ios" {
		t.Errorf("expected platform=ios, got %q", platform)
	}
	if p256 != nil || authKey != nil {
		t.Errorf("expected NULL p256dh/auth for ios, got p256=%v auth=%v", p256, authKey)
	}
}

// TestSubscribePush_AndroidStoresToken — same shape as iOS but
// platform='android'. Kept as its own test so a future platform-
// specific storage rule doesn't silently regress.
func TestSubscribePush_AndroidStoresToken(t *testing.T) {
	db := setupTestDB(t)
	me := seedTestUser(t, db, "me", "me@example.com", "TestPass123")
	ctx := httpctx.WithUserID(context.Background(), me.ID)
	h := &ActivityHandler{DB: db, ReadDB: db}

	_, err := h.SubscribePush(ctx, connectReq(&activityv1.SubscribePushRequest{
		Platform: "android",
		Endpoint: "fcm-reg-token-xyz",
	}, ""))
	if err != nil {
		t.Fatalf("android subscribe: %v", err)
	}
	var platform string
	_ = db.QueryRow(
		"SELECT platform FROM push_subscriptions WHERE endpoint=$1",
		"fcm-reg-token-xyz",
	).Scan(&platform)
	if platform != "android" {
		t.Errorf("expected platform=android, got %q", platform)
	}
}

// TestSubscribePush_NativeWithKeysRejected — passing p256dh/auth on a
// native platform is a client bug; reject loudly so the mobile team
// catches it during development rather than silently saving garbage.
func TestSubscribePush_NativeWithKeysRejected(t *testing.T) {
	db := setupTestDB(t)
	me := seedTestUser(t, db, "me", "me@example.com", "TestPass123")
	ctx := httpctx.WithUserID(context.Background(), me.ID)
	h := &ActivityHandler{DB: db, ReadDB: db}

	p256 := "nope"
	_, err := h.SubscribePush(ctx, connectReq(&activityv1.SubscribePushRequest{
		Platform:  "ios",
		Endpoint:  "apns-token",
		P256DhKey: &p256,
	}, ""))
	if err == nil {
		t.Fatal("expected InvalidArgument when ios subscription sends p256dh_key")
	}
}

// TestSubscribePush_UnknownPlatformRejected — an unknown platform
// string is a client bug; don't silently fall back to 'web'.
func TestSubscribePush_UnknownPlatformRejected(t *testing.T) {
	db := setupTestDB(t)
	me := seedTestUser(t, db, "me", "me@example.com", "TestPass123")
	ctx := httpctx.WithUserID(context.Background(), me.ID)
	h := &ActivityHandler{DB: db, ReadDB: db}

	_, err := h.SubscribePush(ctx, connectReq(&activityv1.SubscribePushRequest{
		Platform: "windowsphone",
		Endpoint: "x",
	}, ""))
	if err == nil {
		t.Fatal("expected InvalidArgument for unknown platform")
	}
}

// TestSubscribePush_EmptyPlatformDefaultsToWeb — the pre-rename web
// bundle doesn't send a platform. Server treats that as web so the
// existing clients keep working across the rollout window.
func TestSubscribePush_EmptyPlatformDefaultsToWeb(t *testing.T) {
	db := setupTestDB(t)
	me := seedTestUser(t, db, "me", "me@example.com", "TestPass123")
	ctx := httpctx.WithUserID(context.Background(), me.ID)
	h := &ActivityHandler{DB: db, ReadDB: db}

	p256 := "legacy-p"
	auth := "legacy-a"
	_, err := h.SubscribePush(ctx, connectReq(&activityv1.SubscribePushRequest{
		Endpoint:  "https://fcm.example/legacy",
		P256DhKey: &p256,
		AuthKey:   &auth,
	}, ""))
	if err != nil {
		t.Fatalf("legacy subscribe: %v", err)
	}

	var platform string
	_ = db.QueryRow(
		"SELECT platform FROM push_subscriptions WHERE endpoint=$1",
		"https://fcm.example/legacy",
	).Scan(&platform)
	if platform != "web" {
		t.Errorf("expected empty platform to default to 'web', got %q", platform)
	}
}

// TestSubscribePush_RejectsAnon — no auth context → 401.
func TestSubscribePush_RejectsAnon(t *testing.T) {
	db := setupTestDB(t)
	h := &ActivityHandler{DB: db, ReadDB: db}
	_, err := h.SubscribePush(context.Background(), connectReq(
		webSubscribeReq("https://fcm.example/abc", "p", "a"), ""))
	if err == nil {
		t.Fatal("expected 401 for anon caller")
	}
}

// TestSubscribePush_WebRequiresKeys — web platform still needs the
// encryption triple; empty p256dh/auth is an error.
func TestSubscribePush_WebRequiresKeys(t *testing.T) {
	db := setupTestDB(t)
	me := seedTestUser(t, db, "me", "me@example.com", "TestPass123")
	ctx := httpctx.WithUserID(context.Background(), me.ID)
	h := &ActivityHandler{DB: db, ReadDB: db}

	_, err := h.SubscribePush(ctx, connectReq(&activityv1.SubscribePushRequest{
		Platform: "web",
		Endpoint: "https://fcm.example/nokeys",
	}, ""))
	if err == nil {
		t.Fatal("expected InvalidArgument for web subscribe without keys")
	}
}

// TestUnsubscribePush_DeletesOwnRow — caller's own subscription
// gets deleted; another user's is left untouched.
func TestUnsubscribePush_DeletesOwnRow(t *testing.T) {
	db := setupTestDB(t)
	me := seedTestUser(t, db, "me", "me@example.com", "TestPass123")
	other := seedTestUser(t, db, "other", "other@example.com", "TestPass123")

	for _, row := range []struct {
		uid      uint
		endpoint string
	}{
		{me.ID, "https://fcm.example/mine"},
		{other.ID, "https://fcm.example/others"},
	} {
		if _, err := db.Exec(
			`INSERT INTO push_subscriptions (user_id, platform, endpoint, p256dh_key, auth_key)
			 VALUES ($1, 'web', $2, 'p', 'a')`,
			row.uid, row.endpoint,
		); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	ctx := httpctx.WithUserID(context.Background(), me.ID)
	h := &ActivityHandler{DB: db, ReadDB: db}

	resp, err := h.UnsubscribePush(ctx, connectReq(&activityv1.UnsubscribePushRequest{
		Endpoint: "https://fcm.example/mine",
	}, ""))
	if err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
	if !resp.Msg.Removed {
		t.Error("expected Removed=true")
	}

	var myCount int
	_ = db.Get(&myCount, "SELECT COUNT(*) FROM push_subscriptions WHERE endpoint=$1", "https://fcm.example/mine")
	if myCount != 0 {
		t.Errorf("expected my row to be deleted, got count=%d", myCount)
	}
	var othersCount int
	_ = db.Get(&othersCount, "SELECT COUNT(*) FROM push_subscriptions WHERE endpoint=$1", "https://fcm.example/others")
	if othersCount != 1 {
		t.Errorf("other user's row should still exist, got count=%d", othersCount)
	}
}

// TestUnsubscribePush_CannotDeleteOthers — an attacker who knows
// another user's endpoint can't delete their subscription.
func TestUnsubscribePush_CannotDeleteOthers(t *testing.T) {
	db := setupTestDB(t)
	me := seedTestUser(t, db, "me", "me@example.com", "TestPass123")
	other := seedTestUser(t, db, "other", "other@example.com", "TestPass123")

	if _, err := db.Exec(
		`INSERT INTO push_subscriptions (user_id, platform, endpoint, p256dh_key, auth_key)
		 VALUES ($1, 'web', 'https://fcm.example/others', 'p', 'a')`,
		other.ID,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ctx := httpctx.WithUserID(context.Background(), me.ID)
	h := &ActivityHandler{DB: db, ReadDB: db}

	resp, err := h.UnsubscribePush(ctx, connectReq(&activityv1.UnsubscribePushRequest{
		Endpoint: "https://fcm.example/others",
	}, ""))
	if err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
	if resp.Msg.Removed {
		t.Error("expected Removed=false when endpoint belongs to another user")
	}
	var count int
	_ = db.Get(&count, "SELECT COUNT(*) FROM push_subscriptions WHERE endpoint=$1", "https://fcm.example/others")
	if count != 1 {
		t.Errorf("expected others' row intact, got count=%d", count)
	}
}

// TestGetPushConfig_ReturnsVapidKey — returns the VAPID_PUBLIC_KEY
// env var, or "" when unset.
func TestGetPushConfig_ReturnsVapidKey(t *testing.T) {
	db := setupTestDB(t)
	me := seedTestUser(t, db, "me", "me@example.com", "TestPass123")
	ctx := httpctx.WithUserID(context.Background(), me.ID)
	h := &ActivityHandler{DB: db, ReadDB: db}

	t.Setenv("VAPID_PUBLIC_KEY", "test-public-key-xyz")

	resp, err := h.GetPushConfig(ctx, connectReq(&activityv1.GetPushConfigRequest{}, ""))
	if err != nil {
		t.Fatalf("GetPushConfig: %v", err)
	}
	if resp.Msg.VapidPublicKey != "test-public-key-xyz" {
		t.Errorf("expected key round-trip, got %q", resp.Msg.VapidPublicKey)
	}
}

// TestGetPushConfig_RejectsAnon — no auth context → 401.
func TestGetPushConfig_RejectsAnon(t *testing.T) {
	db := setupTestDB(t)
	h := &ActivityHandler{DB: db, ReadDB: db}
	_, err := h.GetPushConfig(context.Background(), connectReq(&activityv1.GetPushConfigRequest{}, ""))
	if err == nil {
		t.Fatal("expected 401 for anon caller")
	}
}
