// Command backfill_short_ids populates the `short_id` column on every
// matchups/brackets row that predates migration 014. Safe to re-run: it
// only touches rows where short_id IS NULL.
//
// Usage (from repo root):
//
//	cd api && go run ./cmd/backfill_short_ids
//
// Env vars (reads the same .env as the API via godotenv):
//
//	DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME
//
// Output is one log line per batch + one final summary. Exits 0 when every
// row has a non-NULL short_id, non-zero on unrecoverable errors.
//
// Safety:
//   - Runs in batches of 1000 to avoid a long-running transaction.
//   - Each row is updated via `UPDATE … WHERE id = $1 AND short_id IS NULL`
//     — concurrent creates that beat the backfill don't get clobbered.
//   - SQLSTATE 23505 (unique_violation) triggers a retry with a fresh ID,
//     up to 3 tries per row before giving up on that row and moving on.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	appdb "Matchup/db"

	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

const batchSize = 1000

type tableSpec struct {
	name string // "matchups" / "brackets"
}

var tables = []tableSpec{
	{name: "matchups"},
	{name: "brackets"},
}

func main() {
	if os.Getenv("APP_ENV") != "production" {
		_ = godotenv.Load()
	}

	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		os.Getenv("DB_HOST"), os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"), os.Getenv("DB_PORT"),
	)
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer db.Close()

	total := 0
	for _, tbl := range tables {
		n, err := backfillTable(db, tbl.name)
		if err != nil {
			log.Fatalf("backfill %s: %v", tbl.name, err)
		}
		total += n
		log.Printf("%s: backfilled %d rows", tbl.name, n)
	}
	log.Printf("done. %d rows updated across %d tables.", total, len(tables))
}

// backfillTable walks one table in batches, writing a fresh short_id to
// each row that still has NULL. Returns the number of rows successfully
// updated.
func backfillTable(db *sqlx.DB, table string) (int, error) {
	updated := 0
	for {
		var ids []uint
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err := sqlx.SelectContext(ctx, db, &ids,
			fmt.Sprintf("SELECT id FROM %s WHERE short_id IS NULL ORDER BY id ASC LIMIT $1", table),
			batchSize,
		)
		cancel()
		if err != nil {
			return updated, fmt.Errorf("select batch: %w", err)
		}
		if len(ids) == 0 {
			return updated, nil
		}
		for _, id := range ids {
			if err := updateOne(db, table, id); err != nil {
				log.Printf("%s id=%d: giving up — %v", table, id, err)
				continue
			}
			updated++
		}
		log.Printf("%s: batch of %d done (running total %d)", table, len(ids), updated)
	}
}

// updateOne writes a short_id to one row, retrying on the unlikely event
// of a random-ID collision with an existing value.
func updateOne(db *sqlx.DB, table string, id uint) error {
	for attempt := 0; attempt < 3; attempt++ {
		sid := appdb.GenerateShortID()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := db.ExecContext(ctx,
			fmt.Sprintf("UPDATE %s SET short_id = $1 WHERE id = $2 AND short_id IS NULL", table),
			sid, id,
		)
		cancel()
		if err == nil {
			return nil
		}
		if !appdb.IsUniqueViolation(err) {
			return err
		}
	}
	return fmt.Errorf("collision retries exhausted")
}
