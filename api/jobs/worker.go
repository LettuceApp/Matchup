package jobs

import (
	"context"
	"errors"
	"log"
	"time"
)

// Handler processes one payload pulled off a queue. Returning a non-nil
// error logs the failure and skips the job — there is intentionally no
// retry. Handlers that need durability should write their progress
// somewhere persistent before returning.
type Handler func(ctx context.Context, payload []byte) error

// RunWorker dequeues jobs from queueName in a loop until ctx is
// cancelled. Each job is processed inline (one at a time per worker
// goroutine). Errors are logged so the worker keeps going — the design
// goal is "best effort, never crash the worker over a single bad job."
//
// Callers typically launch one RunWorker goroutine per queue from
// api/cmd/worker. To get more parallelism, launch multiple goroutines
// against the same queue.
func RunWorker(ctx context.Context, queueName string, handler Handler) {
	log.Printf("jobs: worker starting for queue %q", queueName)
	defer log.Printf("jobs: worker stopped for queue %q", queueName)

	for {
		// Cheap pre-check: if the parent context is gone, exit before
		// hitting Redis at all.
		if err := ctx.Err(); err != nil {
			return
		}

		payload, err := Dequeue(ctx, queueName)
		if err != nil {
			if errors.Is(err, ErrQueueClosed) {
				return
			}
			log.Printf("jobs: dequeue error on %q: %v", queueName, err)
			// Don't tight-loop on Redis errors.
			time.Sleep(time.Second)
			continue
		}
		if payload == nil {
			// BRPOP timed out — loop and check ctx again.
			continue
		}

		// Each job gets its own handler context so a hung handler
		// doesn't permanently jam the worker.
		handlerCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := handler(handlerCtx, payload); err != nil {
			log.Printf("jobs: handler error on %q: %v", queueName, err)
			SendToDLQ(ctx, queueName, payload)
		}
		cancel()
	}
}
