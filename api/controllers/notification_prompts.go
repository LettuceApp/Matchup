package controllers

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)


// ScanClosingMatchups emits a `matchup_closing_soon` notification for
// each active matchup whose end_time falls within the next 59-61
// minutes. The 2-minute window around the 60-minute target gives the
// scheduler (which runs every minute) a stable opportunity to fire the
// notification exactly once, even if a single tick is missed. Dedupe
// is via the unique index (user_id, kind, subject_type, subject_id,
// NULL threshold_int) — a second tick in the same window is a no-op.
//
// Two passes:
//  1. Author row. Payload.role = 'author' (CTA: "ready to finalize?").
//  2. One row per distinct non-anonymous voter, minus the author (the
//     UNIQUE index would dedupe anyway, but filtering keeps it cheap).
//     Payload.role = 'voter' (CTA: "vote before it ends").
//
// Anonymous voters can't receive notifications — they have no user
// account to deliver to. The author INSERT runs first so it wins
// the ON CONFLICT race if the author also voted in their own matchup.
func ScanClosingMatchups(ctx context.Context, db *sqlx.DB) error {
	// Pass 1: author.
	var rows []kindPushRow
	err := sqlx.SelectContext(ctx, db, &rows, `
		INSERT INTO public.notifications
			(user_id, kind, subject_type, subject_id, payload, occurred_at)
		SELECT m.author_id,
		       'matchup_closing_soon',
		       'matchup',
		       m.id,
		       jsonb_build_object(
		           'title',    m.title,
		           'role',     'author',
		           'end_time', to_char(m.end_time AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		       ),
		       NOW()
		FROM public.matchups m
		WHERE m.status = 'active'
		  AND m.end_time IS NOT NULL
		  AND m.end_time BETWEEN NOW() + INTERVAL '59 minutes'
		                    AND NOW() + INTERVAL '61 minutes'
		ON CONFLICT DO NOTHING
		RETURNING
			user_id,
			kind,
			COALESCE(payload->>'title', '') AS title,
			payload->>'metric'              AS metric,
			payload->>'threshold'           AS threshold,
			payload->>'role'                AS role,
			subject_type,
			subject_id`)
	if err != nil {
		return fmt.Errorf("scan_closing_matchups (author pass): %w", err)
	}
	publishKindNotifications(ctx, db, rows)

	// Pass 2: voters. Scoped to the same 59-61 min window so we emit
	// at the same scheduler tick as the author row. DISTINCT collapses
	// a voter who swapped items once or twice.
	var voterRows []kindPushRow
	err = sqlx.SelectContext(ctx, db, &voterRows, `
		INSERT INTO public.notifications
			(user_id, kind, subject_type, subject_id, payload, occurred_at)
		SELECT DISTINCT mv.user_id,
		       'matchup_closing_soon',
		       'matchup',
		       m.id,
		       jsonb_build_object(
		           'title',    m.title,
		           'role',     'voter',
		           'end_time', to_char(m.end_time AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		       ),
		       NOW()
		FROM public.matchups m
		JOIN public.matchup_votes mv ON mv.matchup_public_id = m.public_id
		WHERE m.status = 'active'
		  AND m.end_time IS NOT NULL
		  AND m.end_time BETWEEN NOW() + INTERVAL '59 minutes'
		                    AND NOW() + INTERVAL '61 minutes'
		  AND mv.user_id IS NOT NULL
		  AND mv.user_id <> m.author_id
		ON CONFLICT DO NOTHING
		RETURNING
			user_id,
			kind,
			COALESCE(payload->>'title', '') AS title,
			payload->>'metric'              AS metric,
			payload->>'threshold'           AS threshold,
			payload->>'role'                AS role,
			subject_type,
			subject_id`)
	if err != nil {
		return fmt.Errorf("scan_closing_matchups (voter pass): %w", err)
	}
	publishKindNotifications(ctx, db, voterRows)

	return nil
}

// ScanTiesNeedingResolution emits a `tie_needs_resolution` notification
// for each active matchup whose end_time has passed, whose winner is
// not yet set, and whose top vote total is tied across 2+ items. The
// author is the one who has to step in, so they're the notified user.
//
// Tie detection: we compare MAX(votes) with the COUNT of items sharing
// that max. If count >= 2, we have a tie. Using a lateral subquery per
// matchup keeps the expression readable without window functions.
func ScanTiesNeedingResolution(ctx context.Context, db *sqlx.DB) error {
	var rows []kindPushRow
	err := sqlx.SelectContext(ctx, db, &rows, `
		INSERT INTO public.notifications
			(user_id, kind, subject_type, subject_id, payload, occurred_at)
		SELECT m.author_id,
		       'tie_needs_resolution',
		       'matchup',
		       m.id,
		       jsonb_build_object('title', m.title),
		       NOW()
		FROM public.matchups m
		WHERE m.status = 'active'
		  AND m.end_time IS NOT NULL
		  AND m.end_time < NOW()
		  AND m.winner_item_id IS NULL
		  AND (
		      SELECT COUNT(*) FROM public.matchup_items mi
		      WHERE mi.matchup_id = m.id
		        AND mi.votes = (
		            SELECT MAX(votes) FROM public.matchup_items
		            WHERE matchup_id = m.id
		        )
		  ) >= 2
		ON CONFLICT DO NOTHING
		RETURNING
			user_id,
			kind,
			COALESCE(payload->>'title', '') AS title,
			payload->>'metric'              AS metric,
			payload->>'threshold'           AS threshold,
			payload->>'role'                AS role,
			subject_type,
			subject_id`)
	if err != nil {
		return fmt.Errorf("scan_ties_needing_resolution: %w", err)
	}
	publishKindNotifications(ctx, db, rows)
	return nil
}
