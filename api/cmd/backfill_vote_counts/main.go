// backfill_vote_counts reconciles matchup_items.votes against the
// source-of-truth matchup_votes table. Users have reported items
// that display 0% even though they've clicked + the frontend flags
// them as voted; root cause is data drift between the rollup column
// (matchup_items.votes) and the actual vote rows. The current vote
// handler bumps both correctly going forward, but historical writes
// — especially during transient Redis outages or earlier code paths
// that committed the matchup_votes row before bumping the counter —
// left some items with rollups that disagree with the row count.
//
// Once that drift exists, every subsequent click on the same item
// hits the "AlreadyVoted" branch (since the matchup_votes row points
// at that item) and returns the stale rollup, so the item appears
// permanently stuck at its drifted value.
//
// Usage:
//
//	go run ./cmd/backfill_vote_counts            # report only
//	go run ./cmd/backfill_vote_counts --apply    # write the fix
//
// The DRY-RUN default scans every matchup_items row and prints
// `(matchup_id, item_id, label, db_votes, true_count)` for any row
// where the two disagree. Inspect the output, then re-run with
// `--apply` to actually UPDATE the rows. Idempotent — running it a
// second time on a clean DB is a no-op.
//
// Why a cmd instead of a migration: migrations are versioned + run
// once at boot, which is the wrong shape for a recurring repair
// (we may want to run this again after future code regressions).
// A cmd can be invoked manually from a Render shell, an admin
// laptop, or scheduled by a cron worker.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type drift struct {
	matchupID  int64
	itemID     int64
	publicID   string
	label      string
	dbVotes    int64
	trueCount  int64
}

func main() {
	_ = godotenv.Load()

	apply := flag.Bool("apply", false, "Apply the fixes (default: dry-run / report only)")
	flag.Parse()

	dsn, err := buildDSN()
	if err != nil {
		log.Fatalf("could not build DB DSN: %v", err)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("could not open DB: %v", err)
	}
	defer db.Close()

	// One pass over matchup_items, left-joined to a COUNT-aggregated
	// matchup_votes subquery keyed by matchup_item_public_id. Skip
	// rows whose `kind` is 'skip' — those don't count toward an
	// item's tally even though they reference the matchup. The COUNT
	// is COALESCE'd to zero so items with no votes still get the
	// comparison.
	rows, err := db.Query(`
		SELECT
			mi.matchup_id,
			mi.id,
			mi.public_id,
			COALESCE(mi.item, ''),
			COALESCE(mi.votes, 0)::bigint,
			COALESCE(v.cnt, 0)::bigint
		FROM matchup_items mi
		LEFT JOIN (
			SELECT matchup_item_public_id, COUNT(*) AS cnt
			FROM matchup_votes
			WHERE kind = 'pick' AND matchup_item_public_id IS NOT NULL
			GROUP BY matchup_item_public_id
		) v ON v.matchup_item_public_id = mi.public_id
		ORDER BY mi.matchup_id, mi.id
	`)
	if err != nil {
		log.Fatalf("scan failed: %v", err)
	}
	defer rows.Close()

	var drifts []drift
	scanned := 0
	for rows.Next() {
		var d drift
		if err := rows.Scan(&d.matchupID, &d.itemID, &d.publicID, &d.label, &d.dbVotes, &d.trueCount); err != nil {
			log.Fatalf("row scan failed: %v", err)
		}
		scanned++
		if d.dbVotes != d.trueCount {
			drifts = append(drifts, d)
		}
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("rows err: %v", err)
	}

	fmt.Printf("Scanned %d matchup_items rows.\n", scanned)
	fmt.Printf("Found %d rows with drift between matchup_items.votes and COUNT(matchup_votes).\n\n", len(drifts))
	if len(drifts) == 0 {
		return
	}

	// Print the drift table — concise enough to read in a terminal
	// but specific enough that someone can spot-check the matchup
	// ID + label before applying.
	fmt.Printf("%-12s %-12s %-32s %-10s %-10s %s\n",
		"MATCHUP_ID", "ITEM_ID", "PUBLIC_ID", "DB_VOTES", "TRUE_CNT", "LABEL")
	fmt.Println(strings.Repeat("-", 110))
	for _, d := range drifts {
		fmt.Printf("%-12d %-12d %-32s %-10d %-10d %s\n",
			d.matchupID, d.itemID, d.publicID, d.dbVotes, d.trueCount, truncate(d.label, 40))
	}

	if !*apply {
		fmt.Println("\nDry-run mode. Re-run with --apply to write the fixes.")
		return
	}

	// Apply phase — single UPDATE per row, wrapped in a transaction
	// so a transient DB error halts the whole batch rather than
	// leaving us half-fixed.
	tx, err := db.Begin()
	if err != nil {
		log.Fatalf("begin tx: %v", err)
	}
	// matchup_items has no updated_at column (baseline schema: id,
	// public_id, matchup_id, item, votes + later image_path/user_id).
	stmt, err := tx.Prepare("UPDATE matchup_items SET votes = $1 WHERE id = $2")
	if err != nil {
		tx.Rollback()
		log.Fatalf("prepare: %v", err)
	}
	defer stmt.Close()

	for _, d := range drifts {
		if _, err := stmt.Exec(d.trueCount, d.itemID); err != nil {
			tx.Rollback()
			log.Fatalf("update item %d: %v", d.itemID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		log.Fatalf("commit: %v", err)
	}
	fmt.Printf("\nApplied fixes to %d rows.\n", len(drifts))
}

// truncate is a tiny helper for the report table — keeps long item
// labels from wrapping the terminal line.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// buildDSN mirrors api/server.go: prefer DATABASE_URL in prod;
// otherwise stitch one together from the DB_* env vars.
func buildDSN() (string, error) {
	if url := strings.TrimSpace(os.Getenv("DATABASE_URL")); url != "" {
		return url, nil
	}
	host := envOrDefault("DB_HOST", "localhost")
	port := envOrDefault("DB_PORT", "5432")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	name := os.Getenv("DB_NAME")
	if user == "" || name == "" {
		return "", fmt.Errorf("DB_USER and DB_NAME must be set (or supply DATABASE_URL)")
	}
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, name,
	), nil
}

func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
