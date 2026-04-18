package controllers

import (
	"context"
	"fmt"
	"time"

	"Matchup/cache"
	"Matchup/models"
)

// AdvanceExpiredBrackets finds every timer-based bracket whose current round
// has expired and advances each one through a single call to
// advanceBracketInternal. Returns the number of brackets that were actually
// advanced and the first error encountered (if any). A non-nil error does
// not abort the loop — every bracket is given its own attempt, and errors
// are logged via context so a single stuck bracket can't starve the rest.
//
// This is the replacement for the former Dagster `advance_expired_brackets`
// sensor + dynamic-mapped job (orchestration/sensors.py + ops.py). The
// scheduler package (api/scheduler) calls this method every 10 seconds.
//
// Design notes:
//   - The SELECT is the same query the Dagster sensor used. Polling the
//     primary is cheap: `brackets` is small (hundreds of rows, not millions)
//     and the WHERE clause hits indexed columns.
//   - Each advance runs inside its own transaction with SELECT FOR UPDATE
//     (see advanceBracketInternal). Concurrent schedulers are therefore
//     safe — the second one simply waits on the row lock and then observes
//     the updated round_ends_at, which fails the `currentRound != nextRound`
//     guard inside advanceBracketInternal. No idempotency token needed.
//   - We deliberately do NOT early-exit on first error. One malformed
//     bracket should not block unrelated brackets on the same tick.
func (s *Server) AdvanceExpiredBrackets(ctx context.Context) (int, error) {
	if s == nil || s.DB == nil {
		return 0, fmt.Errorf("AdvanceExpiredBrackets: server DB is nil")
	}

	rows, err := s.DB.QueryxContext(ctx, `
		SELECT id
		FROM brackets
		WHERE status = 'active'
		  AND advance_mode = 'timer'
		  AND round_ends_at IS NOT NULL
		  AND round_ends_at <= NOW()
		ORDER BY id ASC
	`)
	if err != nil {
		return 0, fmt.Errorf("AdvanceExpiredBrackets: select expired: %w", err)
	}
	defer rows.Close()

	var ids []uint
	for rows.Next() {
		var id uint
		if err := rows.Scan(&id); err != nil {
			return 0, fmt.Errorf("AdvanceExpiredBrackets: scan: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("AdvanceExpiredBrackets: rows err: %w", err)
	}
	rows.Close()

	if len(ids) == 0 {
		return 0, nil
	}

	advanced := 0
	var firstErr error
	for _, id := range ids {
		// Honour cancellation between brackets so SIGTERM drains quickly.
		select {
		case <-ctx.Done():
			return advanced, ctx.Err()
		default:
		}

		// Distributed lock — prevents the scheduler and request-path
		// tryAdvanceIfExpired from racing into the same FOR UPDATE row lock.
		if cache.Client != nil {
			lockKey := fmt.Sprintf("advance_lock:%d", id)
			acquired, _ := cache.Client.SetNX(ctx, lockKey, "1", 30*time.Second).Result()
			if !acquired {
				continue
			}
		}

		var bracket models.Bracket
		if _, err := bracket.FindBracketByID(s.DB, id); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("load bracket %d: %w", id, err)
			}
			continue
		}

		ok, err := s.advanceBracketInternal(s.DB, &bracket)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("advance bracket %d: %w", id, err)
			}
			continue
		}
		if ok {
			advanced++
		}
	}

	return advanced, firstErr
}
