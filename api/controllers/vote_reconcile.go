package controllers

// vote_reconcile.go houses the single source of truth for the
// matchup_items.votes ↔ matchup_votes reconciliation SQL. Three
// callers share this function:
//
//	1. api/cmd/backfill_vote_counts   — one-shot manual repair, dev box.
//	2. api/scheduler.jobReconcileVoteCounts — nightly cron at 04:30 UTC.
//	3. POST /admin/jobs/reconcile-vote-counts — admin-only HTTP trigger,
//	   for environments without a shell (Render's free tier, etc.).
//
// Keeping the SQL in one place means the three paths can never
// drift — a fix in one fixes all. Each caller is responsible for
// its own auth + timeouts; this helper just runs the SQL and
// returns the row count.

import (
	"context"
	"database/sql"
)

// reconcileVoteCountsSQL rewrites matchup_items.votes from the
// authoritative COUNT(matchup_votes) per item, only touching rows
// whose rollup currently disagrees. Idempotent on a clean DB —
// running it again is a no-op because the WHERE clause filters
// already-correct rows.
//
// NOTE: matchup_items has no updated_at column (see migration 001
// baseline — id, public_id, matchup_id, item, votes, plus
// image_path + user_id from later migrations). Earlier drafts of
// this SQL set `updated_at = NOW()` and failed with `column
// "updated_at" of relation "matchup_items" does not exist`. Don't
// add it back without a schema change.
const reconcileVoteCountsSQL = `
WITH true_counts AS (
	SELECT mi.id AS item_id,
	       COALESCE(v.cnt, 0)::int AS true_count
	FROM matchup_items mi
	LEFT JOIN (
		SELECT matchup_item_public_id, COUNT(*) AS cnt
		FROM matchup_votes
		WHERE kind = 'pick' AND matchup_item_public_id IS NOT NULL
		GROUP BY matchup_item_public_id
	) v ON v.matchup_item_public_id = mi.public_id
)
UPDATE matchup_items mi
SET votes = tc.true_count
FROM true_counts tc
WHERE mi.id = tc.item_id
  AND mi.votes <> tc.true_count
`

// execer is the minimum surface this helper needs — accepts both
// *sqlx.DB and *sql.DB so callers can pass whichever they hold.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

// ReconcileVoteCounts runs the rollup ↔ source-of-truth repair SQL
// and returns the number of rows it had to fix. Zero is the healthy-
// system answer; a non-zero number means drift existed and was
// repaired. Errors from the DB pass straight through — the caller
// chooses whether to log, retry, or fail the request.
func ReconcileVoteCounts(ctx context.Context, db execer) (int64, error) {
	result, err := db.ExecContext(ctx, reconcileVoteCountsSQL)
	if err != nil {
		return 0, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}
