// Command cron is Matchup's background scheduler. It replaces the old
// Dagster orchestration stack (orchestration/*.py) with a single Go
// process that runs every recurring workload using the same packages the
// API uses: controllers for bracket advancement, db for the connection
// pool, cache for the Redis client.
//
// Run it as its own K8s deployment (k8s/cron/deployment.yaml) or via
// docker-compose alongside the API. One replica is plenty — every
// workload is safe under SELECT FOR UPDATE (bracket advance) or
// idempotent SQL (everything else). If you ever need to scale, add a
// Redis-based leader lock first.
//
// The binary catches SIGINT / SIGTERM so K8s rolling updates drain
// cleanly: outstanding cron jobs finish, the 10s advance ticker stops,
// then the process exits.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"Matchup/controllers"
	"Matchup/scheduler"

	"github.com/joho/godotenv"
)

func main() {
	// Local dev convenience — production mounts env via the K8s secret,
	// so the .env file is only loaded when it exists.
	if os.Getenv("APP_ENV") != "production" {
		_ = godotenv.Load()
	}

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetPrefix("cron: ")

	// Reuse controllers.Server.Initialize so the cron pod gets the same
	// DB pool tuning, optional read-replica fallback, Redis client, and
	// S3 singleton the API uses. The router it builds internally is
	// ignored — nothing in this binary calls ListenAndServe.
	server := &controllers.Server{}
	server.Initialize(
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_NAME"),
	)
	if server.DB == nil {
		log.Fatal("server.DB is nil after Initialize; check DB env vars")
	}

	// The cron pod runs 6 workloads (none concurrently thanks to
	// SkipIfStillRunning), so it needs far fewer connections than the
	// API's default 25. Right-sizing avoids exhausting Postgres
	// max_connections when the API HPA scales up.
	server.DB.SetMaxOpenConns(5)
	server.DB.SetMaxIdleConns(2)
	if server.ReadDB != nil && server.ReadDB != server.DB {
		server.ReadDB.SetMaxOpenConns(5)
		server.ReadDB.SetMaxIdleConns(2)
	}

	defer server.DB.Close()
	if server.ReadDB != nil && server.ReadDB != server.DB {
		defer server.ReadDB.Close()
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	sched := scheduler.New(server)
	log.Println("boot complete; handing off to scheduler.Run")
	if err := sched.Run(ctx); err != nil {
		log.Fatalf("scheduler.Run returned error: %v", err)
	}
	log.Println("exit: clean shutdown")
}
