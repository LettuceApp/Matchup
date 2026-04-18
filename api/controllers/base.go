package controllers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"Matchup/cache"
	appdb "Matchup/db"
	"Matchup/middlewares"
	"Matchup/security"

	aws2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jmoiron/sqlx"
)

type Server struct {
	// DB is the primary read/write connection pool. All writes, all
	// `SELECT FOR UPDATE`, and any read that lives inside a write
	// transaction must go through DB.
	DB *sqlx.DB
	// ReadDB is a connection pool aimed at the read replica
	// (k8s/postgres/replica-deployment.yaml). For dev/test, where no
	// replica exists, ReadDB is set to the same handle as DB so call
	// sites don't need to know whether a replica is configured.
	ReadDB *sqlx.DB
	// S3Client is a process-wide singleton initialized in Initialize().
	// Constructing s3.NewFromConfig is expensive (it walks the AWS
	// credential chain and resolves the endpoint), so doing it once at
	// startup keeps avatar/image upload latency predictable. Handlers
	// that need it (UserHandler, MatchupHandler) get it injected via
	// initializeConnectRoutes — they do not call NewFromConfig themselves.
	// Nil if AWS config could not be loaded; handlers must guard.
	S3Client *s3.Client
	Router   chi.Router
}

// initS3Client builds the singleton s3.Client at startup. Returns nil if
// the AWS config can't be loaded — uploads will then fail fast at the
// handler level instead of crashing the entire server boot. Pulled out
// of Initialize() so it stays unit-testable in isolation.
func initS3Client() *s3.Client {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-2"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var (
		cfg aws2.Config
		err error
	)
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")
	if accessKey != "" && secretKey != "" {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, sessionToken)),
		)
	} else {
		cfg, err = config.LoadDefaultConfig(ctx, config.WithRegion(region))
	}
	if err != nil {
		log.Printf("warning: AWS config load failed; uploads will be unavailable: %v", err)
		return nil
	}
	return s3.NewFromConfig(cfg)
}

// ===============================
// SECURE ADMIN SEEDING
// ===============================
func seedAdmin(db *sqlx.DB) error {
	adminEmail := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL")))
	adminPassword := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD"))

	if adminEmail == "" || adminPassword == "" {
		log.Println("[seedAdmin] ADMIN_EMAIL or ADMIN_PASSWORD not set — skipping admin creation.")
		return nil
	}

	ctx := context.Background()
	var existingID uint
	var isAdmin bool
	err := db.QueryRowContext(ctx,
		"SELECT id, is_admin FROM users WHERE email = $1", adminEmail,
	).Scan(&existingID, &isAdmin)

	if err == sql.ErrNoRows {
		log.Println("[seedAdmin] Creating initial admin:", adminEmail)

		hashedPw, err := security.Hash(adminPassword)
		if err != nil {
			return err
		}
		publicID := appdb.GeneratePublicID()
		username := strings.Split(adminEmail, "@")[0]

		_, err = db.ExecContext(ctx,
			`INSERT INTO users (public_id, username, email, password, avatar_path, bio, is_admin, is_private, followers_count, following_count, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, '', '', true, false, 0, 0, NOW(), NOW())`,
			publicID, username, adminEmail, string(hashedPw),
		)
		if err != nil {
			log.Printf("[seedAdmin] failed to create admin: %v\n", err)
			return err
		}
		return nil
	}

	if err == nil && !isAdmin {
		log.Println("[seedAdmin] Ensuring admin flag is set for:", adminEmail)
		_, err = db.ExecContext(ctx,
			"UPDATE users SET is_admin = true WHERE id = $1", existingID,
		)
		return err
	}

	return err
}

// ===============================
// SERVER INITIALIZATION
// ===============================
func (server *Server) Initialize(DbUser, DbPassword, DbPort, DbHost, DbName string) {
	var dsn string

	if strings.EqualFold(os.Getenv("APP_ENV"), "production") {
		dsn = os.Getenv("DATABASE_URL")
		if dsn != "" && !strings.Contains(dsn, "sslmode=") {
			if strings.Contains(dsn, "?") {
				dsn += "&sslmode=require"
			} else {
				dsn += "?sslmode=require"
			}
		}
	} else {
		dsn = fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=UTC",
			DbHost, DbUser, DbPassword, DbName, DbPort,
		)
	}

	db, err := appdb.Connect(dsn)
	if err != nil {
		log.Fatalf("Cannot connect to Postgres: %v", err)
	}
	server.DB = db

	// Optional read replica. READ_DATABASE_URL points at the replica
	// service (k8s service `postgres-replica`). When unset — common
	// during local dev — read traffic transparently falls back to the
	// primary so handlers don't need a code-path branch.
	if readDSN := os.Getenv("READ_DATABASE_URL"); readDSN != "" {
		readDB, err := appdb.ConnectRead(readDSN)
		if err != nil {
			log.Printf("warning: could not connect to read replica, falling back to primary: %v", err)
			server.ReadDB = db
		} else {
			server.ReadDB = readDB
		}
	} else {
		server.ReadDB = db
	}

	// Redis init (safe failure)
	if err := cache.InitFromEnv(); err != nil {
		log.Printf("warning: could not connect to redis: %v", err)
	}

	// Build the S3 client once. Failure is non-fatal — uploads will
	// reject at the handler level if S3Client is nil — but every other
	// part of the API still comes up.
	server.S3Client = initS3Client()

	// SECURE ADMIN CREATION
	if err := seedAdmin(server.DB); err != nil {
		log.Printf("error seeding admin user: %v\n", err)
	}

	r := chi.NewRouter()

	r.Use(chiMiddleware.RealIP)
	r.Use(chiMiddleware.Recoverer)

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Anon-Id", "X-Internal-Key", "Connect-Protocol-Version", "Connect-Timeout-Ms"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// ETag middleware. Self-disables for non-GET methods (so all the
	// ConnectRPC POSTs pass through untouched) and for streaming
	// responses (SSE). The wins land on /health and any future plain
	// GETs we add — small but free.
	r.Use(middlewares.ETagMiddleware())

	// Read-after-write: if the client just performed a write, route
	// reads to the primary for 5s to avoid stale-read surprises.
	r.Use(middlewares.ReadAfterWriteMiddleware())

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"backend running"}`))
	})

	server.Router = r
	initializeConnectRoutes(r, server.DB, server.ReadDB, server.S3Client)
}

func (server *Server) Run(addr string) {
	log.Fatal(http.ListenAndServe(addr, server.Router))
}

func splitCSV(input string) []string {
	parts := strings.Split(input, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
