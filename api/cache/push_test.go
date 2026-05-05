package cache

import (
	"encoding/json"
	"errors"
	"testing"
)

// These unit tests cover payload-shaping + dead-signal detection —
// the pure-JSON / pure-logic bits of the push backends. The live
// send paths (Web Push HTTP/2 post, APNS2 client, FCM Admin SDK) are
// exercised by manual live-smoke against real creds; mocking those
// three network clients is tons of test scaffolding for little value.

func TestWebPayload_Shape(t *testing.T) {
	p := PushPayload{
		Title: "@alice mentioned you",
		Body:  "ready to pick sides?",
		URL:   "https://matchup.test/users/alice/matchup/xyz",
		Tag:   "mention:xyz",
	}
	shape := webPayload(p)

	b, err := json.Marshal(shape)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]string
	_ = json.Unmarshal(b, &got)

	want := map[string]string{
		"title": "@alice mentioned you",
		"body":  "ready to pick sides?",
		"url":   "https://matchup.test/users/alice/matchup/xyz",
		"tag":   "mention:xyz",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("web payload[%s] = %q, want %q", k, got[k], v)
		}
	}
}

func TestWebPayload_OmitsEmptyOptionals(t *testing.T) {
	// Verify `omitempty` serialisation: missing URL + Tag shouldn't
	// leak as empty strings in the JSON, keeping the SW's
	// `payload.url || '/'` fallback branch honest.
	p := PushPayload{Title: "t", Body: "b"}
	b, err := json.Marshal(webPayload(p))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if contains(s, "\"url\"") || contains(s, "\"tag\"") {
		t.Errorf("expected url + tag to be omitted, got %s", s)
	}
}

func TestAPNSPayload_Shape(t *testing.T) {
	p := PushPayload{
		Title: "🎯 100 votes",
		Body:  "Best Rapper Alive",
		URL:   "https://matchup.test/",
		Tag:   "milestone_reached",
	}
	b, err := json.Marshal(apnsPayload(p))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Parse back as the canonical APNS envelope to assert every key
	// is in the right place — `aps.alert.title`, etc.
	var parsed struct {
		Aps struct {
			Alert struct {
				Title string `json:"title"`
				Body  string `json:"body"`
			} `json:"alert"`
			Sound string `json:"sound"`
		} `json:"aps"`
		URL string `json:"url"`
		Tag string `json:"tag"`
	}
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("parse apns: %v", err)
	}
	if parsed.Aps.Alert.Title != p.Title {
		t.Errorf("aps.alert.title = %q, want %q", parsed.Aps.Alert.Title, p.Title)
	}
	if parsed.Aps.Alert.Body != p.Body {
		t.Errorf("aps.alert.body = %q, want %q", parsed.Aps.Alert.Body, p.Body)
	}
	if parsed.Aps.Sound != "default" {
		t.Errorf("aps.sound = %q, want \"default\"", parsed.Aps.Sound)
	}
	if parsed.URL != p.URL {
		t.Errorf("url = %q, want %q", parsed.URL, p.URL)
	}
	if parsed.Tag != p.Tag {
		t.Errorf("tag = %q, want %q", parsed.Tag, p.Tag)
	}
}

func TestFCMMessage_Shape(t *testing.T) {
	p := PushPayload{
		Title: "⏰ Closes in ~1 hour",
		Body:  "Best Rapper Alive — ready to finalize?",
		URL:   "https://matchup.test/",
		Tag:   "matchup_closing_soon",
	}
	msg := fcmMessage("fcm-reg-token", p)

	if msg.Token != "fcm-reg-token" {
		t.Errorf("message.Token = %q, want fcm-reg-token", msg.Token)
	}
	if msg.Notification == nil {
		t.Fatal("message.Notification must be populated")
	}
	if msg.Notification.Title != p.Title {
		t.Errorf("Notification.Title = %q, want %q", msg.Notification.Title, p.Title)
	}
	if msg.Notification.Body != p.Body {
		t.Errorf("Notification.Body = %q, want %q", msg.Notification.Body, p.Body)
	}
	// `url` + `tag` land in the Data map so the Android client can
	// route on them from the receiver.
	if msg.Data["url"] != p.URL {
		t.Errorf("Data[url] = %q, want %q", msg.Data["url"], p.URL)
	}
	if msg.Data["tag"] != p.Tag {
		t.Errorf("Data[tag] = %q, want %q", msg.Data["tag"], p.Tag)
	}
}

// TestIsDeadFCM_NegativeCases — nil error + arbitrary non-FCM errors
// must NOT be classified as dead. (The positive cases — matching the
// actual FCM error shape — require `firebase.google.com/go/v4/internal`
// which isn't exported; those paths are covered by live smoke tests
// against the Firebase Admin SDK.)
func TestIsDeadFCM_NegativeCases(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"nil error", nil},
		{"random error", errors.New("boom")},
		{"network timeout lookalike", errors.New("context deadline exceeded")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if isDeadFCM(tc.err) {
				t.Errorf("isDeadFCM(%v) returned true; expected false", tc.err)
			}
		})
	}
}

// contains is a tiny substring helper used by the omitempty check —
// keeps the test free of import cycles for such a small need.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
