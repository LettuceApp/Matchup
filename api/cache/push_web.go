package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// Web Push sender — the original push backend, extracted unchanged
// from the pre-v2 webpush.go. Uses VAPID-signed HTTP requests to the
// browser's push service (Mozilla autopush / Google FCM-for-web / etc).
//
// Required env:
//
//	VAPID_PUBLIC_KEY   URL-safe base64 EC public key
//	VAPID_PRIVATE_KEY  URL-safe base64 EC private key
//	VAPID_SUBJECT      contact URL, usually "mailto:<admin email>"
//
// When any are missing, sendWebPush returns (false, nil) — no send,
// no prune. Matches the "dev without creds shouldn't explode" contract
// the dispatcher expects from every backend.
//
// Dead signals: HTTP 410 Gone (user unsubscribed) or 404 Not Found
// (endpoint no longer known to the push service). Any other non-2xx
// is treated as transient — the row stays, we retry next publish.

func sendWebPush(ctx context.Context, s pushSubscriptionRow, p PushPayload) (bool, error) {
	publicKey := os.Getenv("VAPID_PUBLIC_KEY")
	privateKey := os.Getenv("VAPID_PRIVATE_KEY")
	subject := os.Getenv("VAPID_SUBJECT")
	if publicKey == "" || privateKey == "" || subject == "" {
		return false, nil // silent no-op on dev machines
	}

	// Web Push rows MUST have p256dh + auth — the handler enforces this
	// at subscribe time (see activity_connect_handler.go). Defensive
	// check anyway; a missing key means the row was manually mucked
	// with + we should skip rather than panic on a nil deref.
	if s.P256dhKey == nil || s.AuthKey == nil || *s.P256dhKey == "" || *s.AuthKey == "" {
		return true, nil // prune — the row is useless for Web Push
	}

	body, err := json.Marshal(webPayload(p))
	if err != nil {
		return false, fmt.Errorf("web push marshal: %w", err)
	}

	sub := &webpush.Subscription{
		Endpoint: s.Endpoint,
		Keys: webpush.Keys{
			P256dh: *s.P256dhKey,
			Auth:   *s.AuthKey,
		},
	}
	opts := &webpush.Options{
		Subscriber:      subject,
		VAPIDPublicKey:  publicKey,
		VAPIDPrivateKey: privateKey,
		TTL:             30, // seconds — time-sensitive
	}
	resp, err := webpush.SendNotificationWithContext(ctx, body, sub, opts)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusGone, http.StatusNotFound:
		return true, nil
	case http.StatusCreated, http.StatusAccepted, http.StatusOK:
		return false, nil
	default:
		return false, fmt.Errorf("web push: unexpected status %d", resp.StatusCode)
	}
}

// webPayloadShape mirrors the JSON object the service worker decodes
// in public/sw.js. Kept lowercase-tagged so the SW doesn't need to
// care about Go casing conventions.
type webPayloadShape struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url,omitempty"`
	Tag   string `json:"tag,omitempty"`
}

func webPayload(p PushPayload) webPayloadShape {
	return webPayloadShape{
		Title: p.Title,
		Body:  p.Body,
		URL:   p.URL,
		Tag:   p.Tag,
	}
}
