package cache

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
)

// Push publisher — one dispatcher, three delivery channels.
//
// Callers get a single symbol (PublishPushToUser) + a single payload
// type (PushPayload). The dispatcher looks at each subscription's
// `platform` column and routes to sendWebPush / sendAPNS / sendFCM
// accordingly. Per-backend credentials come from env vars; when a
// backend's creds are absent, its sender no-ops silently — dev
// machines without VAPID/APNS/FCM don't crash, they just skip.
//
// Dead-token handling: each sender returns (dead bool, err error).
// dead=true means the browser/device has unsubscribed (or the app
// was uninstalled). The dispatcher DELETEs those rows inline so
// subsequent publishes don't retry them.
//
// Transient errors (network, 5xx) are logged and the row is kept.
// The next notification for that user retries the same token.

// PushPayload is the wire-neutral shape every sender adapts into its
// protocol's specific layout. Four fields cover today's priority
// kinds (mentions, milestones, closing-soon, tie-resolution); the
// struct is deliberately flat to avoid versioning a schema the
// service worker / iOS / Android all need to agree on.
type PushPayload struct {
	Title string // Notification headline.
	Body  string // One-line preview.
	URL   string // Deep link the app opens on tap.
	Tag   string // Collapses duplicates on the notification tray.
}

// pushSubscriptionRow mirrors the row we fetch on every dispatch.
// P256dhKey / AuthKey are nullable since v2 — only Web Push rows use
// them. Pointer-to-string lets us distinguish "empty key" from "NULL".
type pushSubscriptionRow struct {
	ID        int64   `db:"id"`
	Platform  string  `db:"platform"`
	Endpoint  string  `db:"endpoint"`
	P256dhKey *string `db:"p256dh_key"`
	AuthKey   *string `db:"auth_key"`
}

// PublishPushToUser fans out a payload to every push subscription the
// user has, across every platform they've enabled. Safe to call when
// the user has zero subs, when specific backends are unconfigured,
// and when Postgres is down (returns the error; callers generally
// log and continue).
func PublishPushToUser(ctx context.Context, db *sqlx.DB, userID uint, payload PushPayload) error {
	if userID == 0 {
		return nil
	}

	var subs []pushSubscriptionRow
	if err := sqlx.SelectContext(ctx, db, &subs, `
		SELECT id, platform, endpoint, p256dh_key, auth_key
		  FROM push_subscriptions
		 WHERE user_id = $1
	`, userID); err != nil {
		return fmt.Errorf("push: select subs: %w", err)
	}
	if len(subs) == 0 {
		return nil
	}

	stamps := make([]int64, 0, len(subs))
	for _, s := range subs {
		dead, err := dispatchOne(ctx, s, payload)
		if err != nil {
			// Transient — keep the row, try again next time.
			log.Printf("push: send (%s) failed for sub %d: %v", s.Platform, s.ID, err)
			continue
		}
		if dead {
			// Unsubscribed / app uninstalled / token rotated away.
			// Prune so the next publish for this user doesn't retry.
			if _, err := db.ExecContext(ctx,
				`DELETE FROM push_subscriptions WHERE id = $1`, s.ID,
			); err != nil {
				log.Printf("push: prune dead sub %d: %v", s.ID, err)
			}
			continue
		}
		stamps = append(stamps, s.ID)
	}

	if len(stamps) > 0 {
		query, args, err := sqlx.In(
			`UPDATE push_subscriptions SET last_used_at = ? WHERE id IN (?)`,
			time.Now().UTC(), stamps,
		)
		if err == nil {
			_, _ = db.ExecContext(ctx, db.Rebind(query), args...)
		}
	}
	return nil
}

// dispatchOne is the per-sub platform switch. Kept separate from
// PublishPushToUser so the dispatcher logic (select + prune + stamp)
// is trivial to unit-test with a stub dispatchOne.
func dispatchOne(ctx context.Context, s pushSubscriptionRow, payload PushPayload) (bool, error) {
	switch s.Platform {
	case "web":
		return sendWebPush(ctx, s, payload)
	case "ios":
		return sendAPNS(ctx, s.Endpoint, payload)
	case "android":
		return sendFCM(ctx, s.Endpoint, payload)
	default:
		// Unknown platform — the CHECK constraint in migration 021
		// makes this unreachable in production, but a defensive skip
		// keeps the dispatcher happy if someone bypasses the handler.
		return false, fmt.Errorf("unknown platform %q", s.Platform)
	}
}

// VAPIDPublicKey returns the configured VAPID public key, or "" when
// the server has no keypair configured. Exported so GetPushConfig
// can return it to the web client without re-reading env.
func VAPIDPublicKey() string {
	return os.Getenv("VAPID_PUBLIC_KEY")
}
