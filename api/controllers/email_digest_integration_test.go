//go:build integration

package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"Matchup/mailer"

	"github.com/go-chi/chi/v5"
)

// fakeDigestMailer captures every SendWeeklyDigest call instead of
// shipping to SendGrid. Swapped into mailer.DigestMail via t.Cleanup.
type fakeDigestMailer struct {
	sent []mailer.DigestPayload
}

func (f *fakeDigestMailer) SendWeeklyDigest(p mailer.DigestPayload, _, _ string) (*mailer.EmailResponse, error) {
	f.sent = append(f.sent, p)
	return &mailer.EmailResponse{Status: 200, RespBody: "fake"}, nil
}

func swapDigestMail(t *testing.T) *fakeDigestMailer {
	t.Helper()
	// SendGrid creds must be non-empty or SendEmailDigests bails early.
	t.Setenv("SENDGRID_FROM", "noreply@test.local")
	t.Setenv("SENDGRID_API_KEY", "test-key")
	t.Setenv("API_SECRET", "test-hmac-secret")

	fake := &fakeDigestMailer{}
	prev := mailer.DigestMail
	mailer.DigestMail = fake
	t.Cleanup(func() { mailer.DigestMail = prev })
	return fake
}

// TestSendEmailDigests_HappyPath — a user who opted into the digest
// (default) and has real activity in the last week gets one email
// with the correct headline counts. The user's last_sent_at stamps
// after send so a second run is a no-op.
func TestSendEmailDigests_HappyPath(t *testing.T) {
	db := setupTestDB(t)
	fake := swapDigestMail(t)

	owner := seedTestUser(t, db, "owner", "owner@example.com", "TestPass123")
	fan := seedTestUser(t, db, "fan", "fan@example.com", "TestPass123")
	m := seedTestMatchup(t, db, owner.ID, "Some matchup")

	// 2 likes + 1 comment on owner's matchup within the digest window.
	if _, err := db.Exec(
		`INSERT INTO likes (user_id, matchup_id, created_at, updated_at) VALUES ($1, $2, NOW(), NOW())`,
		fan.ID, m.ID,
	); err != nil {
		t.Fatalf("seed like: %v", err)
	}
	// A second like from a seeded third user so NewLikes=2.
	other := seedTestUser(t, db, "other", "other@example.com", "TestPass123")
	if _, err := db.Exec(
		`INSERT INTO likes (user_id, matchup_id, created_at, updated_at) VALUES ($1, $2, NOW(), NOW())`,
		other.ID, m.ID,
	); err != nil {
		t.Fatalf("seed like 2: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO comments (user_id, matchup_id, body, created_at, updated_at) VALUES ($1, $2, 'good take', NOW(), NOW())`,
		fan.ID, m.ID,
	); err != nil {
		t.Fatalf("seed comment: %v", err)
	}

	sent, err := SendEmailDigests(context.Background(), db)
	if err != nil {
		t.Fatalf("SendEmailDigests: %v", err)
	}

	// 3 users were eligible: owner (has activity), fan (has seen
	// activity on the matchup they liked, which isn't theirs so no
	// stats), other (same). fan + other have empty payloads so they
	// get skipped. Only owner ships.
	if sent != 1 {
		t.Fatalf("expected 1 email sent, got %d", sent)
	}
	if len(fake.sent) != 1 {
		t.Fatalf("fake mailer captured %d payloads, want 1", len(fake.sent))
	}
	payload := fake.sent[0]
	if payload.ToEmail != "owner@example.com" {
		t.Errorf("expected owner to be the recipient, got %s", payload.ToEmail)
	}
	if payload.NewLikes != 2 {
		t.Errorf("expected NewLikes=2, got %d", payload.NewLikes)
	}
	if payload.NewComments != 1 {
		t.Errorf("expected NewComments=1, got %d", payload.NewComments)
	}
	if payload.UnsubscribeURL == "" {
		t.Error("expected UnsubscribeURL to be set")
	}

	// Second run within cooldown — owner is skipped, 0 additional sends.
	sent2, err := SendEmailDigests(context.Background(), db)
	if err != nil {
		t.Fatalf("rerun: %v", err)
	}
	if sent2 != 0 {
		t.Errorf("rerun should send 0, got %d", sent2)
	}
}

// TestSendEmailDigests_SkipsMutedUsers — a user whose email_digest
// pref is false must not receive an email, even if they'd otherwise
// have qualifying activity.
func TestSendEmailDigests_SkipsMutedUsers(t *testing.T) {
	db := setupTestDB(t)
	fake := swapDigestMail(t)

	owner := seedTestUser(t, db, "owner", "owner@example.com", "TestPass123")
	fan := seedTestUser(t, db, "fan", "fan@example.com", "TestPass123")
	m := seedTestMatchup(t, db, owner.ID, "Muted's matchup")

	// Mute the owner's digest pref.
	if _, err := db.Exec(
		`UPDATE users SET notification_prefs = jsonb_set(notification_prefs, '{email_digest}', 'false'::jsonb) WHERE id = $1`,
		owner.ID,
	); err != nil {
		t.Fatalf("mute prefs: %v", err)
	}

	// Give them activity that WOULD have qualified.
	if _, err := db.Exec(
		`INSERT INTO likes (user_id, matchup_id, created_at, updated_at) VALUES ($1, $2, NOW(), NOW())`,
		fan.ID, m.ID,
	); err != nil {
		t.Fatalf("seed like: %v", err)
	}

	sent, err := SendEmailDigests(context.Background(), db)
	if err != nil {
		t.Fatalf("SendEmailDigests: %v", err)
	}
	if sent != 0 {
		t.Errorf("expected 0 sent to muted user, got %d", sent)
	}
	if len(fake.sent) != 0 {
		t.Errorf("fake mailer captured %d payloads, want 0", len(fake.sent))
	}
}

// TestSendEmailDigests_SkipsQuietWeeks — a user with the digest
// enabled but zero activity this week must not receive an email.
func TestSendEmailDigests_SkipsQuietWeeks(t *testing.T) {
	db := setupTestDB(t)
	fake := swapDigestMail(t)

	seedTestUser(t, db, "quiet", "quiet@example.com", "TestPass123")

	sent, err := SendEmailDigests(context.Background(), db)
	if err != nil {
		t.Fatalf("SendEmailDigests: %v", err)
	}
	if sent != 0 {
		t.Errorf("expected 0 sent for quiet user, got %d", sent)
	}
	if len(fake.sent) != 0 {
		t.Errorf("fake mailer captured %d payloads, want 0", len(fake.sent))
	}
}

// TestEmailUnsubscribeHandler_ValidTokenFlipsPref — the token returned
// by BuildUnsubscribeURL verifies successfully and flips the user's
// email_digest pref to false.
func TestEmailUnsubscribeHandler_ValidTokenFlipsPref(t *testing.T) {
	db := setupTestDB(t)
	t.Setenv("API_SECRET", "test-hmac-secret")
	u := seedTestUser(t, db, "subbed", "sub@example.com", "TestPass123")

	url := BuildUnsubscribeURL(u.ID)
	// Extract the token segment — everything after the last "/".
	token := url[len("http://127.0.0.1:8888/email/unsubscribe/"):]

	// Route the request through chi so the URL param extraction works.
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("token", token)
	req := httptest.NewRequest(http.MethodGet, "/email/unsubscribe/"+token, nil).
		WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	EmailUnsubscribeHandler(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	// Verify DB state: email_digest should now be false.
	var prefs []byte
	if err := db.Get(&prefs, "SELECT notification_prefs FROM users WHERE id=$1", u.ID); err != nil {
		t.Fatalf("read prefs: %v", err)
	}
	decoded := decodeNotificationPrefs(prefs)
	if decoded["email_digest"] {
		t.Errorf("expected email_digest=false after unsubscribe, got true")
	}
}

// TestEmailUnsubscribeHandler_BadToken returns 400 and leaves prefs
// untouched.
func TestEmailUnsubscribeHandler_BadToken(t *testing.T) {
	db := setupTestDB(t)
	t.Setenv("API_SECRET", "test-hmac-secret")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("token", "42.deadbeef")
	req := httptest.NewRequest(http.MethodGet, "/email/unsubscribe/42.deadbeef", nil).
		WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	EmailUnsubscribeHandler(db)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad token, got %d", w.Code)
	}
}
