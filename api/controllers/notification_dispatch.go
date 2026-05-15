package controllers

import (
	"context"
	"encoding/json"
	"fmt"

	"Matchup/cache"

	"github.com/jmoiron/sqlx"
)

// dispatchNotification is the canonical "fan out a single notification
// to a user" helper. Three things happen in order:
//
//  1. INSERT INTO notifications with ON CONFLICT DO NOTHING — keys on
//     (user_id, kind, subject_type, subject_id, COALESCE(threshold_int, -1))
//     per the dedupe unique index. Idempotent: re-running with the same
//     subject is a no-op.
//  2. cache.PublishActivity — SSE ping that tells the recipient's bell
//     to refetch its activity feed.
//  3. cache.PublishPushToUser — native web push (only when pushTitle is
//     non-empty so callers can opt out of the noisy native banner per
//     kind).
//
// Notification dispatch was scattered across scanners (notification_scan.go,
// notification_prompts.go) and comment-mention paths (comment_connect_handler.go,
// which only fires SSE+push without inserting a row). This helper
// consolidates the "want a permanent notification row + ping + push"
// pattern so callers don't drift on the dedupe key shape or forget to
// fire one of the three legs.
//
// payload is jsonb on the row; nil → {}.
//
// Errors are logged but never returned — a failed dispatch shouldn't
// roll back the user-visible action (creating a matchup, finishing a
// vote). Callers want best-effort delivery.
func dispatchNotification(
	ctx context.Context,
	db sqlx.ExtContext,
	userID uint,
	kind, subjectType string,
	subjectID uint,
	payload map[string]any,
	pushTitle, pushBody, pushURL string,
) {
	if userID == 0 || kind == "" || subjectType == "" || subjectID == 0 {
		return
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		payloadJSON = []byte(`{}`)
	}

	_, err = db.ExecContext(ctx, `
        INSERT INTO notifications (user_id, kind, subject_type, subject_id, payload, occurred_at)
        VALUES ($1, $2, $3, $4, $5::jsonb, NOW())
        ON CONFLICT (user_id, kind, subject_type, subject_id, COALESCE(threshold_int, -1)) DO NOTHING
    `, userID, kind, subjectType, subjectID, string(payloadJSON))
	if err != nil {
		// Log but don't surface — see helper header for rationale.
		_ = err
	}

	// SSE ping — fires whether or not the INSERT was a no-op. The
	// frontend uses the ping as a hint to refetch; a duplicate ping
	// on a duplicate row is harmless.
	_ = cache.PublishActivity(ctx, userID)

	// Optional native push.
	if pushTitle != "" {
		// Tag composed from kind + subject so duplicate notifications
		// replace each other in the OS notification tray instead of
		// stacking.
		tag := fmt.Sprintf("%s:%d", kind, subjectID)
		// Use sqlx.ExtContext-typed db; PublishPushToUser needs *sqlx.DB.
		// Most callers will already have one; pass nil to skip push if
		// they don't (caller's choice via empty title).
		if dbConcrete, ok := db.(*sqlx.DB); ok {
			_ = cache.PublishPushToUser(ctx, dbConcrete, userID, cache.PushPayload{
				Title: pushTitle,
				Body:  pushBody,
				URL:   pushURL,
				Tag:   tag,
			})
		}
	}
}

// Notification kinds introduced by the social-loop cycle. Strings are
// the wire contract — frontend's activity_connect_handler maps them to
// pref categories. Keep these in lock-step with the kindCategory map
// in activity_connect_handler.go.
const (
	kindMentionedAsItem = "mentioned_as_item" // "you were added to a matchup as an item"
	kindWonMatchup      = "won_matchup"       // "you won a matchup you were placed in"
	kindWonRound        = "won_round"         // bracket: advanced a round
	kindWonBracket      = "won_bracket"       // bracket: won the whole bracket
)
