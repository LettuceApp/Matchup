package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/token"
)

// APNS sender — iOS / iPadOS / macOS-native delivery via Apple Push
// Notification service.
//
// Required env (all five or none — sender no-ops when any is missing):
//
//	APNS_KEY_ID     10-char key identifier from Apple Developer portal
//	APNS_TEAM_ID    10-char team identifier
//	APNS_BUNDLE_ID  app bundle (e.g. "com.matchup.app") — APNS topic
//	APNS_KEY_P8     PEM contents of the .p8 EC private key
//	APNS_ENV        "sandbox" (dev/TestFlight) or "production"
//
// The apns2 library manages the HTTP/2 connection pool + token
// (JWT-per-connection) rotation internally. One client per process
// via sync.Once.
//
// Dead signals — apns2 reports these on the response:
//
//	StatusCode 410 Gone            user uninstalled / token expired
//	Reason "Unregistered"          same semantics (410 + Reason)
//	Reason "BadDeviceToken"        malformed or stale token
//	Reason "DeviceTokenNotForTopic" token belongs to a different app
//
// All four collapse into dead=true so the dispatcher prunes the row.

var (
	apnsOnce   sync.Once
	apnsClient *apns2.Client
	apnsTopic  string
	apnsReady  bool
)

func initAPNSOnce() {
	apnsOnce.Do(func() {
		keyID := os.Getenv("APNS_KEY_ID")
		teamID := os.Getenv("APNS_TEAM_ID")
		bundleID := os.Getenv("APNS_BUNDLE_ID")
		keyP8 := os.Getenv("APNS_KEY_P8")
		if keyID == "" || teamID == "" || bundleID == "" || keyP8 == "" {
			return // apnsReady stays false; sender will no-op
		}

		authKey, err := token.AuthKeyFromBytes([]byte(keyP8))
		if err != nil {
			// Don't crash the server; just log and leave APNS off.
			// Operators see this on first boot + can fix the key.
			fmt.Fprintf(os.Stderr, "apns: parse key failed: %v\n", err)
			return
		}
		tok := &token.Token{
			AuthKey: authKey,
			KeyID:   keyID,
			TeamID:  teamID,
		}
		client := apns2.NewTokenClient(tok)
		if os.Getenv("APNS_ENV") == "production" {
			client = client.Production()
		} else {
			client = client.Development()
		}
		apnsClient = client
		apnsTopic = bundleID
		apnsReady = true
	})
}

func sendAPNS(ctx context.Context, deviceToken string, p PushPayload) (bool, error) {
	initAPNSOnce()
	if !apnsReady {
		return false, nil
	}
	if deviceToken == "" {
		// Malformed row — don't retry forever.
		return true, nil
	}

	body, err := json.Marshal(apnsPayload(p))
	if err != nil {
		return false, fmt.Errorf("apns marshal: %w", err)
	}
	notification := &apns2.Notification{
		DeviceToken: deviceToken,
		Topic:       apnsTopic,
		Payload:     body,
	}

	resp, err := apnsClient.PushWithContext(ctx, notification)
	if err != nil {
		return false, err
	}

	// Dead-token signals: either the HTTP 410 or any of the four
	// "token is gone / wrong" reason codes Apple documents. Collapse
	// all of them into dead=true.
	if resp.StatusCode == http.StatusGone {
		return true, nil
	}
	switch resp.Reason {
	case "Unregistered", "BadDeviceToken", "DeviceTokenNotForTopic":
		return true, nil
	}
	if !resp.Sent() {
		return false, fmt.Errorf("apns: status=%d reason=%q", resp.StatusCode, resp.Reason)
	}
	return false, nil
}

// apnsPayloadShape is the canonical APNS JSON envelope. `aps` is the
// Apple-required root; we tuck `url` + `tag` as top-level siblings so
// the iOS client can read them from the `userInfo` dict without
// digging into `aps`.
type apnsPayloadShape struct {
	Aps apnsAps `json:"aps"`
	URL string  `json:"url,omitempty"`
	Tag string  `json:"tag,omitempty"`
}

type apnsAps struct {
	Alert apnsAlert `json:"alert"`
	Sound string    `json:"sound,omitempty"`
}

type apnsAlert struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

func apnsPayload(p PushPayload) apnsPayloadShape {
	return apnsPayloadShape{
		Aps: apnsAps{
			Alert: apnsAlert{Title: p.Title, Body: p.Body},
			Sound: "default",
		},
		URL: p.URL,
		Tag: p.Tag,
	}
}
