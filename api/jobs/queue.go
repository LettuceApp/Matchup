// Package jobs provides a Redis-backed background job queue with
// bounded queue length and a dead-letter queue for failed jobs.
//
// All payloads are arbitrary bytes — usually JSON. Producers call
// Enqueue from the API process; the worker (api/cmd/worker) calls
// RunWorker for each queue it cares about.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"Matchup/cache"

	"github.com/redis/go-redis/v9"
)

// MaxQueueLen is the maximum number of jobs allowed in a single queue.
// Enqueue rejects new jobs once the queue reaches this length. The
// cap prevents unbounded Redis memory growth when a consumer is down
// or a downstream dependency (SendGrid, etc.) is unreachable.
const MaxQueueLen int64 = 10000

// MaxDLQLen is the maximum number of failed jobs retained in each
// queue's dead-letter queue. Oldest entries are trimmed on every push.
const MaxDLQLen int64 = 1000

// queueKey is the Redis key for a named queue. Keeping the prefix in one
// place makes it easy to spot in `redis-cli MONITOR` and to namespace
// against other Redis usage in the project.
func queueKey(name string) string {
	return "matchup:jobs:" + name
}

func dlqKey(name string) string {
	return queueKey(name) + ":dlq"
}

// ErrQueueClosed is returned by Dequeue when the underlying context is
// cancelled before a job arrives. Workers use this to know they should
// shut down cleanly rather than treat the cancellation as a failure.
var ErrQueueClosed = errors.New("jobs: queue closed")

// ErrQueueFull is returned by Enqueue when the queue already has
// MaxQueueLen entries. Callers should either retry later or fall back
// to inline processing.
var ErrQueueFull = errors.New("jobs: queue is full")

// Enqueue pushes a job onto the named queue. The payload is opaque to
// the queue itself — the worker that consumes the queue is responsible
// for decoding it.
//
// Enqueue uses LPUSH so a matching BRPOP on the consumer side gives
// FIFO order. The queue is capped at MaxQueueLen entries; exceeding the
// cap returns ErrQueueFull rather than growing without bound.
func Enqueue(ctx context.Context, queueName string, payload []byte) error {
	if cache.Client == nil {
		return fmt.Errorf("jobs: redis client not initialized")
	}
	qLen, err := cache.Client.LLen(ctx, queueKey(queueName)).Result()
	if err != nil {
		return fmt.Errorf("jobs: queue length check: %w", err)
	}
	if qLen >= MaxQueueLen {
		return ErrQueueFull
	}
	return cache.Client.LPush(ctx, queueKey(queueName), payload).Err()
}

// SendToDLQ pushes a failed job payload to the dead-letter queue for the
// named queue. The DLQ is capped at MaxDLQLen entries via LTRIM so it
// never grows unbounded. Errors are logged but not returned — the DLQ
// is a best-effort diagnostic aid, not a critical path.
func SendToDLQ(ctx context.Context, queueName string, payload []byte) {
	if cache.Client == nil {
		return
	}
	pipe := cache.Client.Pipeline()
	pipe.LPush(ctx, dlqKey(queueName), payload)
	pipe.LTrim(ctx, dlqKey(queueName), 0, MaxDLQLen-1)
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("jobs: dlq write failed for %q: %v", queueName, err)
	}
}

// Dequeue blocks until a job is available on the named queue or the
// context is cancelled. The block timeout is short (5s) so the worker
// can poll the context regularly and exit promptly on shutdown.
//
// Returns ErrQueueClosed when the context is cancelled. Returns
// (nil, nil) when the BRPOP times out without a job — the caller
// should loop and try again.
func Dequeue(ctx context.Context, queueName string) ([]byte, error) {
	if cache.Client == nil {
		return nil, fmt.Errorf("jobs: redis client not initialized")
	}
	res, err := cache.Client.BRPop(ctx, 5*time.Second, queueKey(queueName)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// BRPOP timed out without a job — not a real error.
			return nil, nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrQueueClosed
		}
		return nil, err
	}
	// BRPOP returns [key, value]; we only need the value.
	if len(res) < 2 {
		return nil, nil
	}
	return []byte(res[1]), nil
}
