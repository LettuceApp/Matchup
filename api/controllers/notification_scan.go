package controllers

import (
	"context"
	"fmt"
	"strings"

	"Matchup/cache"

	"github.com/jmoiron/sqlx"
)

// pingAffectedUsers publishes per-user activity events for every row
// returned by RETURNING user_id on a scanner INSERT. Swallowing errors
// is intentional — SSE push is best-effort; the notifications row is
// committed before we get here.
func pingAffectedUsers(ctx context.Context, userIDs []uint) {
	for _, uid := range userIDs {
		_ = cache.PublishActivity(ctx, uid)
	}
}

// kindPushRow is the shape every scanner's RETURNING decays into so we
// can reuse one publisher loop. Title comes from payload.title; metric
// + threshold + role are nil for kinds that don't emit them (PG returns
// NULL for a missing jsonb key, which sqlx pulls into *string as nil).
type kindPushRow struct {
	UserID    uint    `db:"user_id"`
	Kind      string  `db:"kind"`
	Title     string  `db:"title"`
	Metric    *string `db:"metric"`
	Threshold *string `db:"threshold"`
	Role      *string `db:"role"`
}

// publishKindNotifications handles SSE + Web Push fanout for the
// scheduler-emitted priority kinds. Every row gets an SSE ping so the
// bell refetches; rows whose kind is in the priority-push set also get
// a native browser notification.
//
// URL strategy: we deliberately point every push at the home page
// rather than a deep link. The scanners don't have the subject's
// author_username handy without a second JOIN, and the home page's
// bell surfaces the new row within a second of landing — good enough
// for a "something happened" prompt.
func publishKindNotifications(ctx context.Context, db *sqlx.DB, rows []kindPushRow) {
	home := strings.TrimRight(mailerAppBaseURL(), "/")
	for _, r := range rows {
		_ = cache.PublishActivity(ctx, r.UserID)
		title, body := composePushCopy(r)
		if title == "" {
			continue
		}
		_ = cache.PublishPushToUser(ctx, db, r.UserID, cache.PushPayload{
			Title: title,
			Body:  body,
			URL:   home,
			Tag:   r.Kind,
		})
	}
}

// composePushCopy picks the native-notification title + body for each
// kind. Empty title means "don't push" (falls through to SSE only).
func composePushCopy(r kindPushRow) (string, string) {
	switch r.Kind {
	case "milestone_reached":
		metric := "engagement"
		if r.Metric != nil {
			metric = *r.Metric
		}
		threshold := ""
		if r.Threshold != nil {
			threshold = *r.Threshold
		}
		return fmt.Sprintf("🎯 %s %s", threshold, metric),
			r.Title
	case "matchup_closing_soon":
		// Voter vs author framing — the scanner tagged it in payload.role.
		if r.Role != nil && *r.Role == "voter" {
			return "⏰ Closes in ~1 hour",
				r.Title + " — lock in your pick."
		}
		return "⏰ Closes in ~1 hour",
			r.Title + " — ready to finalize?"
	case "tie_needs_resolution":
		return "⚠ Tied matchup", r.Title + " — pick a winner."
	default:
		return "", ""
	}
}

// milestoneThresholds are the round-number crossings we notify on. Ten
// is a deliberately low floor so the feature has something to say early;
// 10k is the ceiling because beyond that every +1 feels the same.
var milestoneThresholds = []int{10, 100, 1000, 10000}

// ScanMilestones scans denormalised counters for milestone crossings and
// inserts one `notifications` row per (user, subject, threshold) combo
// that has newly hit a round-number threshold.
//
// Idempotency: every INSERT uses `ON CONFLICT DO NOTHING` against the
// unique index `idx_notifications_dedupe` (user_id, kind, subject_type,
// subject_id, COALESCE(threshold_int, -1)) established by migration 016.
// Re-running the scanner every 15 minutes is safe — rows that already
// exist are silently skipped. The scanner does NOT attempt to backfill
// milestones that were crossed before the scanner first ran; the first
// pass will emit a burst of notifications for all already-crossed
// thresholds, and every subsequent pass is quiet unless new counts push
// content across a threshold.
//
// Metrics in scope:
//
//	matchup_votes   — SUM(matchup_items.votes) per matchup, emitted to matchup author
//	matchup_likes   — matchups.likes_count, emitted to matchup author
//	bracket_likes   — brackets.likes_count, emitted to bracket author
//	user_followers  — users.followers_count, emitted to the user themselves
//
// Thresholds live on the package for test visibility.
//
// Lives in controllers/ rather than scheduler/ so the integration tests
// (which live in the controllers package) can call it directly without
// creating a circular import (scheduler imports controllers).
func ScanMilestones(ctx context.Context, db *sqlx.DB) error {
	var lastErr error

	for _, t := range milestoneThresholds {
		// Matchup votes: SUM(matchup_items.votes) per matchup, keyed to
		// matchup.author_id. matchup_items.votes is a display counter
		// (owner votes count toward it) — we accept that owner-inflated
		// matchups can trip the milestone. If that becomes a problem we
		// can switch to a net-votes subquery.
		var rows []kindPushRow
		err := sqlx.SelectContext(ctx, db, &rows, `
			INSERT INTO public.notifications
				(user_id, kind, subject_type, subject_id, threshold_int, payload, occurred_at)
			SELECT m.author_id,
			       'milestone_reached',
			       'matchup',
			       m.id,
			       $1,
			       jsonb_build_object(
			           'metric', 'votes',
			           'threshold', ($1::integer)::text,
			           'title', m.title
			       ),
			       NOW()
			FROM public.matchups m
			JOIN (
				SELECT matchup_id, COALESCE(SUM(votes), 0) AS total
				FROM public.matchup_items
				GROUP BY matchup_id
				HAVING COALESCE(SUM(votes), 0) >= $1
			) v ON v.matchup_id = m.id
			ON CONFLICT DO NOTHING
			RETURNING
				user_id,
				kind,
				COALESCE(payload->>'title', '') AS title,
				payload->>'metric'              AS metric,
				payload->>'threshold'           AS threshold,
				payload->>'role'                AS role`, t)
		if err != nil {
			lastErr = fmt.Errorf("matchup_votes %d: %w", t, err)
		}
		publishKindNotifications(ctx, db, rows)

		// Matchup likes — denormalised counter, single-JOIN milestone.
		rows = nil
		err = sqlx.SelectContext(ctx, db, &rows, `
			INSERT INTO public.notifications
				(user_id, kind, subject_type, subject_id, threshold_int, payload, occurred_at)
			SELECT m.author_id,
			       'milestone_reached',
			       'matchup',
			       m.id,
			       $1,
			       jsonb_build_object(
			           'metric', 'likes',
			           'threshold', ($1::integer)::text,
			           'title', m.title
			       ),
			       NOW()
			FROM public.matchups m
			WHERE m.likes_count >= $1
			ON CONFLICT DO NOTHING
			RETURNING
				user_id,
				kind,
				COALESCE(payload->>'title', '') AS title,
				payload->>'metric'              AS metric,
				payload->>'threshold'           AS threshold,
				payload->>'role'                AS role`, t)
		if err != nil {
			lastErr = fmt.Errorf("matchup_likes %d: %w", t, err)
		}
		publishKindNotifications(ctx, db, rows)

		// Bracket likes — same shape.
		rows = nil
		err = sqlx.SelectContext(ctx, db, &rows, `
			INSERT INTO public.notifications
				(user_id, kind, subject_type, subject_id, threshold_int, payload, occurred_at)
			SELECT b.author_id,
			       'milestone_reached',
			       'bracket',
			       b.id,
			       $1,
			       jsonb_build_object(
			           'metric', 'likes',
			           'threshold', ($1::integer)::text,
			           'title', b.title
			       ),
			       NOW()
			FROM public.brackets b
			WHERE b.likes_count >= $1
			ON CONFLICT DO NOTHING
			RETURNING
				user_id,
				kind,
				COALESCE(payload->>'title', '') AS title,
				payload->>'metric'              AS metric,
				payload->>'threshold'           AS threshold,
				payload->>'role'                AS role`, t)
		if err != nil {
			lastErr = fmt.Errorf("bracket_likes %d: %w", t, err)
		}
		publishKindNotifications(ctx, db, rows)

		// User followers. subject_id = u.id; user sees the milestone on
		// their own profile. Title falls back to username because users
		// don't have a "title" per se.
		rows = nil
		err = sqlx.SelectContext(ctx, db, &rows, `
			INSERT INTO public.notifications
				(user_id, kind, subject_type, subject_id, threshold_int, payload, occurred_at)
			SELECT u.id,
			       'milestone_reached',
			       'user',
			       u.id,
			       $1,
			       jsonb_build_object(
			           'metric', 'followers',
			           'threshold', ($1::integer)::text,
			           'title', u.username
			       ),
			       NOW()
			FROM public.users u
			WHERE u.followers_count >= $1
			ON CONFLICT DO NOTHING
			RETURNING
				user_id,
				kind,
				COALESCE(payload->>'title', '') AS title,
				payload->>'metric'              AS metric,
				payload->>'threshold'           AS threshold,
				payload->>'role'                AS role`, t)
		if err != nil {
			lastErr = fmt.Errorf("user_followers %d: %w", t, err)
		}
		publishKindNotifications(ctx, db, rows)
	}

	return lastErr
}
