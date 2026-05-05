package cache

import (
	"context"
	"fmt"
	"os"
	"sync"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

// FCM sender — Android-native delivery (and iOS-via-FCM, but we route
// iOS through APNS directly to avoid the extra hop).
//
// Required env:
//
//	FCM_SERVICE_ACCOUNT_JSON  the raw JSON contents of a Firebase
//	                          service-account credential. Download
//	                          from Firebase console →
//	                          Project Settings → Service accounts.
//
// One Firebase messaging client per process via sync.Once. The
// Firebase SDK manages OAuth2 token refresh internally — we don't
// need a background goroutine.
//
// Dead signals, per firebase/go/v4/messaging:
//
//	IsUnregistered(err)       token has been revoked / app uninstalled
//	IsInvalidArgument(err)    malformed token (typo, truncated, etc.)
//	IsSenderIDMismatch(err)   token belongs to a different Firebase app

var (
	fcmOnce   sync.Once
	fcmClient *messaging.Client
	fcmReady  bool
)

func initFCMOnce() {
	fcmOnce.Do(func() {
		creds := os.Getenv("FCM_SERVICE_ACCOUNT_JSON")
		if creds == "" {
			return // fcmReady stays false; sender no-ops
		}
		// Firebase's NewApp needs a context for the creds fetch; the
		// init context outlives only this call so Background is fine.
		ctx := context.Background()
		app, err := firebase.NewApp(ctx, nil,
			option.WithCredentialsJSON([]byte(creds)),
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fcm: firebase.NewApp: %v\n", err)
			return
		}
		client, err := app.Messaging(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fcm: app.Messaging: %v\n", err)
			return
		}
		fcmClient = client
		fcmReady = true
	})
}

func sendFCM(ctx context.Context, registrationToken string, p PushPayload) (bool, error) {
	initFCMOnce()
	if !fcmReady {
		return false, nil
	}
	if registrationToken == "" {
		return true, nil // malformed row
	}

	msg := fcmMessage(registrationToken, p)
	if _, err := fcmClient.Send(ctx, msg); err != nil {
		if isDeadFCM(err) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// fcmMessage composes the messaging.Message from a PushPayload. Kept
// as a pure function so unit tests can assert the wire shape without
// hitting the network.
func fcmMessage(token string, p PushPayload) *messaging.Message {
	return &messaging.Message{
		Token: token,
		Notification: &messaging.Notification{
			Title: p.Title,
			Body:  p.Body,
		},
		// Android surfaces `data` as a Map<String,String> in the app's
		// FCM receiver; url+tag land there for the client to route on.
		Data: map[string]string{
			"url": p.URL,
			"tag": p.Tag,
		},
	}
}

// isDeadFCM centralises the "token is gone" signal across Firebase's
// error taxonomy. Kept exported-private so the unit test can assert
// on canned errors without reaching into this file.
func isDeadFCM(err error) bool {
	return messaging.IsUnregistered(err) ||
		messaging.IsInvalidArgument(err) ||
		messaging.IsSenderIDMismatch(err)
}
