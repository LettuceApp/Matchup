// Command worker is the background job runner.
//
// It connects to the same Redis the API uses, registers a handler per
// queue it cares about, and blocks until the process is killed. Run it
// as a separate K8s deployment (k8s/worker/deployment.yaml) so a slow
// worker can never starve the API request path.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"Matchup/cache"
	appdb "Matchup/db"
	"Matchup/jobs"
	"Matchup/jobs/handlers"

	"github.com/joho/godotenv"
)

func main() {
	// Local dev convenience — production runs without a .env file.
	if os.Getenv("APP_ENV") != "production" {
		_ = godotenv.Load()
	}

	if err := cache.InitFromEnv(); err != nil {
		log.Fatalf("worker: redis init failed: %v", err)
	}

	// The flush_votes job needs a real DB handle to write counter
	// updates back to matchup_items. Other (queue-only) handlers don't
	// touch Postgres directly, so we only open the pool when there is
	// a non-empty DATABASE_URL — keeping local dev viable for the
	// queue-only flow.
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatalf("worker: DATABASE_URL is required")
	}
	db, err := appdb.Connect(dsn)
	if err != nil {
		log.Fatalf("worker: database connect failed: %v", err)
	}

	// Worker runs 1 email queue goroutine + 1 flush_votes ticker.
	// 5 conns is ample; keeps the connection budget tight so the
	// API pods (25 each) have room when HPA scales up.
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)

	defer db.Close()

	// Catch SIGINT/SIGTERM so K8s rolling updates drain cleanly. The
	// context cascades down to every RunWorker goroutine via the
	// shared parent.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Each queue gets its own goroutine. Add new queues here as
	// handlers come online — keep this list short, every entry is a
	// promise the worker will keep up with the queue.
	type registration struct {
		name    string
		handler jobs.Handler
	}
	registrations := []registration{
		{name: handlers.EmailQueue, handler: handlers.HandleEmail},
	}

	var wg sync.WaitGroup
	for _, reg := range registrations {
		wg.Add(1)
		go func(r registration) {
			defer wg.Done()
			jobs.RunWorker(ctx, r.name, r.handler)
		}(reg)
	}

	// flush_votes is tick-driven, not queue-driven, so it lives in its
	// own goroutine outside the registrations loop. It drains pending
	// vote-count deltas from Redis into matchup_items every few seconds.
	wg.Add(1)
	go func() {
		defer wg.Done()
		handlers.RunFlushVotes(ctx, db)
	}()

	log.Printf("worker: started, %d queue(s) registered + flush_votes ticker", len(registrations))
	wg.Wait()
	log.Println("worker: shutdown complete")
}
