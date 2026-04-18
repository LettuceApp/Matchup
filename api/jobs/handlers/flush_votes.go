package handlers

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"Matchup/cache"

	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
)

// VoteDeltaKeyPrefix is re-exported from the cache package where the
// canonical constant lives. Both the producer (controllers.voteDeltaKey)
// and this consumer reference the same source of truth.
const VoteDeltaKeyPrefix = cache.VoteDeltaKeyPrefix

// FlushInterval is how often RunFlushVotes wakes up and drains pending
// vote deltas. 5s is a deliberate compromise:
//   - short enough that vote counts in the DB stay fresh enough for
//     analytics and search
//   - long enough that a hot matchup gets meaningful batching (one
//     UPDATE per item per 5s instead of one UPDATE per vote)
const FlushInterval = 5 * time.Second

// RunFlushVotes runs a periodic ticker that drains pending vote deltas
// from Redis and applies them to the matchup_items table. It is meant
// to be launched as its own goroutine from cmd/worker — there is no
// queue involved, the work is purely tick-driven.
//
// The function blocks until ctx is cancelled. On cancellation it logs
// and returns; the caller (cmd/worker) is responsible for waiting on
// any wait group it added the goroutine to.
func RunFlushVotes(ctx context.Context, db *sqlx.DB) {
	log.Printf("flush_votes: started, interval=%s", FlushInterval)
	defer log.Printf("flush_votes: stopped")

	ticker := time.NewTicker(FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// One last drain on shutdown so any deltas accumulated in
			// the final tick window aren't lost.
			drainCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := flushVotesOnce(drainCtx, db); err != nil {
				log.Printf("flush_votes: final drain error: %v", err)
			}
			cancel()
			return
		case <-ticker.C:
			if err := flushVotesOnce(ctx, db); err != nil {
				log.Printf("flush_votes: tick error: %v", err)
			}
		}
	}
}

// flushVotesOnce performs a single SCAN+drain pass over the
// votes:item:* keyspace. It is exported as an unexported helper rather
// than inlined so the shutdown path can call it with its own context.
//
// The flow per key:
//  1. SCAN finds the key (cursor-based, so safe under concurrent writes)
//  2. GETSET ... 0 atomically reads the current delta and zeros it
//  3. UPDATE matchup_items SET votes = GREATEST(votes + $1, 0) WHERE id = $2
//
// Step 2 is the crucial one — between SCAN and the UPDATE the producer
// (VoteItem) might INCR the same key. GETSET ensures we apply exactly
// the value that existed at the moment of the swap, and any subsequent
// INCRs will be picked up on the next tick.
//
// If GETSET returns 0 (no pending delta) we still issue no UPDATE.
// Empty/zero deltas are silently skipped.
func flushVotesOnce(ctx context.Context, db *sqlx.DB) error {
	if cache.Client == nil {
		return errors.New("flush_votes: redis client not initialized")
	}

	var (
		cursor    uint64
		processed int
		flushed   int
	)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		keys, next, err := cache.Client.Scan(ctx, cursor, VoteDeltaKeyPrefix+"*", 100).Result()
		if err != nil {
			return err
		}

		for _, key := range keys {
			if err := ctx.Err(); err != nil {
				return err
			}

			itemID, ok := parseVoteItemID(key)
			if !ok {
				// Stray key with the prefix but a non-numeric tail.
				// Leave it alone — better than silently deleting
				// something a future producer might own.
				continue
			}
			processed++

			// GETSET reads the current value and atomically sets it to
			// "0". Any in-flight INCR/DECR after this point goes onto a
			// fresh value and will be picked up next tick.
			prev, err := cache.Client.GetSet(ctx, key, "0").Result()
			if err != nil {
				if errors.Is(err, redis.Nil) {
					continue
				}
				log.Printf("flush_votes: getset %s: %v", key, err)
				continue
			}

			delta, parseErr := strconv.ParseInt(prev, 10, 64)
			if parseErr != nil {
				log.Printf("flush_votes: bad delta value %q for %s: %v", prev, key, parseErr)
				continue
			}
			if delta == 0 {
				continue
			}

			if _, err := db.ExecContext(ctx,
				"UPDATE matchup_items SET votes = GREATEST(votes + $1, 0) WHERE id = $2",
				delta, itemID,
			); err != nil {
				// Roll the delta back into Redis so we don't lose it.
				// Best-effort — if Redis itself is now down too there
				// is nothing more we can do.
				if rbErr := cache.Client.IncrBy(ctx, key, delta).Err(); rbErr != nil {
					log.Printf("flush_votes: failed to roll back delta for item %d: %v (db err: %v)", itemID, rbErr, err)
				}
				log.Printf("flush_votes: db update for item %d failed: %v", itemID, err)
				continue
			}
			flushed++
		}

		cursor = next
		if cursor == 0 {
			break
		}
	}

	if processed > 0 {
		log.Printf("flush_votes: scanned=%d flushed=%d", processed, flushed)
	}
	return nil
}

// parseVoteItemID extracts the numeric item ID from a "votes:item:<id>"
// Redis key. Returns false on any malformed input so the caller can
// skip the key without crashing.
func parseVoteItemID(key string) (uint, bool) {
	tail := strings.TrimPrefix(key, VoteDeltaKeyPrefix)
	if tail == key {
		return 0, false
	}
	id, err := strconv.ParseUint(tail, 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(id), true
}
