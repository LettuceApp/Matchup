package controllers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"Matchup/cache"
	appdb "Matchup/db"
	"Matchup/security"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jmoiron/sqlx"
	"github.com/twinj/uuid"
)

type Server struct {
	DB     *sqlx.DB
	Router chi.Router
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
		publicID := uuid.NewV4().String()
		username := strings.Split(adminEmail, "@")[0]

		_, err = db.ExecContext(ctx,
			`INSERT INTO users (public_id, username, email, password, is_admin, is_private, followers_count, following_count, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, true, false, 0, 0, NOW(), NOW())`,
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

	// Redis init (safe failure)
	if err := cache.InitFromEnv(); err != nil {
		log.Printf("warning: could not connect to redis: %v", err)
	}

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

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"backend running"}`))
	})

	server.Router = r
	initializeConnectRoutes(r, server.DB)
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
