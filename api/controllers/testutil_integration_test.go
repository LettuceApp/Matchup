//go:build integration

package controllers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"Matchup/auth"
	appdb "Matchup/db"
	"Matchup/models"
	"Matchup/security"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

// testDSN returns the DSN for the docker-compose Postgres instance.
// Override with TEST_DATABASE_URL env var if needed.
func testDSN() string {
	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		return dsn
	}
	host := os.Getenv("TEST_DB_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	user := os.Getenv("TEST_DB_USER")
	if user == "" {
		user = os.Getenv("DB_USER")
	}
	if user == "" {
		user = "cordelljenkins1914"
	}
	pass := os.Getenv("TEST_DB_PASSWORD")
	if pass == "" {
		pass = os.Getenv("DB_PASSWORD")
	}
	if pass == "" {
		pass = "Limitless12345"
	}
	return fmt.Sprintf(
		"host=%s user=%s password=%s dbname=postgres port=5432 sslmode=disable",
		host, user, pass,
	)
}

// setupTestDB creates an isolated test database, runs all goose migrations,
// and returns a connected *sqlx.DB. The database is dropped when the test
// completes. Each test gets its own database so they can run in parallel.
func setupTestDB(t *testing.T) *sqlx.DB {
	t.Helper()

	// Connect to the default "postgres" database to create our test DB.
	adminDB, err := sqlx.Connect("postgres", testDSN())
	if err != nil {
		t.Fatalf("cannot connect to postgres for test setup: %v", err)
	}

	// Create a uniquely-named test database.
	dbName := fmt.Sprintf("matchup_test_%d", time.Now().UnixNano())
	if _, err := adminDB.Exec("CREATE DATABASE " + dbName); err != nil {
		adminDB.Close()
		t.Fatalf("cannot create test database %s: %v", dbName, err)
	}

	// Build DSN for the test database.
	dsn := testDSN()
	dsn = strings.Replace(dsn, "dbname=postgres", "dbname="+dbName, 1)

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		adminDB.Exec("DROP DATABASE " + dbName)
		adminDB.Close()
		t.Fatalf("cannot connect to test database %s: %v", dbName, err)
	}

	// Run goose migrations.
	migrationsDir := "../migrations"
	if err := goose.Up(db.DB, migrationsDir); err != nil {
		db.Close()
		adminDB.Exec("DROP DATABASE " + dbName)
		adminDB.Close()
		t.Fatalf("goose migrations failed: %v", err)
	}

	// Cleanup: drop the test database when the test finishes.
	t.Cleanup(func() {
		db.Close()
		if _, err := adminDB.Exec("DROP DATABASE " + dbName + " WITH (FORCE)"); err != nil {
			log.Printf("warning: could not drop test database %s: %v", dbName, err)
		}
		adminDB.Close()
	})

	return db
}

// seedTestUser creates a user in the test database and returns it.
//
// By default the seeded user is email_verified_at = NOW() — most
// tests treat the user as a fully-functional "has done onboarding"
// account, and the email-verification soft-gate on CreateMatchup /
// CreateBracket / CreateComment would otherwise force every
// integration test to handle verification. Tests that specifically
// need an UNVERIFIED user (the soft-gate + verification-flow tests)
// use seedUnverifiedUser instead.
func seedTestUser(t *testing.T, db *sqlx.DB, username, email, password string) *models.User {
	t.Helper()
	user := seedUnverifiedUser(t, db, username, email, password)
	if _, err := db.Exec("UPDATE users SET email_verified_at = NOW() WHERE id = $1", user.ID); err != nil {
		t.Fatalf("auto-verify seeded user %s: %v", username, err)
	}
	// Reload so the struct mirrors the DB row (so callers reading
	// user.EmailVerifiedAt see it set).
	if err := db.Get(user, "SELECT * FROM users WHERE id = $1", user.ID); err != nil {
		t.Fatalf("reload seeded user %s: %v", username, err)
	}
	return user
}

// seedUnverifiedUser is the raw seed — no email_verified_at stamp.
// Used by tests of the verification soft-gate where the whole point
// is to observe unverified behaviour.
func seedUnverifiedUser(t *testing.T, db *sqlx.DB, username, email, password string) *models.User {
	t.Helper()
	hashedPw, err := security.Hash(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	publicID := appdb.GeneratePublicID()
	now := time.Now()

	var user models.User
	err = db.QueryRowx(
		`INSERT INTO users (public_id, username, email, password, avatar_path, bio, is_admin, is_private, followers_count, following_count, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, '', '', false, false, 0, 0, $5, $5) RETURNING *`,
		publicID, username, email, string(hashedPw), now,
	).StructScan(&user)
	if err != nil {
		t.Fatalf("seed user %s: %v", username, err)
	}
	return &user
}

// seedAdminUser creates an admin user in the test database.
func seedAdminUser(t *testing.T, db *sqlx.DB, username, email, password string) *models.User {
	t.Helper()
	user := seedTestUser(t, db, username, email, password)
	if _, err := db.Exec("UPDATE users SET is_admin = true WHERE id = $1", user.ID); err != nil {
		t.Fatalf("promote to admin: %v", err)
	}
	user.IsAdmin = true
	return user
}

// createTestToken creates a JWT for the given user ID.
func createTestToken(t *testing.T, userID uint) string {
	t.Helper()
	t.Setenv("API_SECRET", "integration-test-secret")
	token, err := auth.CreateToken(userID)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	return token
}

// seedTestMatchup creates a matchup with two items in the test database.
func seedTestMatchup(t *testing.T, db *sqlx.DB, authorID uint, title string) *models.Matchup {
	t.Helper()
	m := models.Matchup{
		Title:    title,
		Content:  "Test matchup content",
		AuthorID: authorID,
		Status:   "published",
		Items: []models.MatchupItem{
			{Item: "Option A"},
			{Item: "Option B"},
		},
	}
	m.Prepare()
	saved, err := m.SaveMatchup(db)
	if err != nil {
		t.Fatalf("seed matchup %s: %v", title, err)
	}
	return saved
}

// seedTestBracket creates a bracket in the test database.
func seedTestBracket(t *testing.T, db *sqlx.DB, authorID uint, title string, size int) *models.Bracket {
	t.Helper()
	b := models.Bracket{
		Title:       title,
		Description: "Test bracket",
		AuthorID:    authorID,
		Size:        size,
		Status:      "draft",
		AdvanceMode: "manual",
		Visibility:  "public",
	}
	b.Prepare()
	saved, err := b.SaveBracket(db)
	if err != nil {
		t.Fatalf("seed bracket %s: %v", title, err)
	}
	return saved
}

// seedTestComment creates a comment on a matchup.
func seedTestComment(t *testing.T, db *sqlx.DB, userID, matchupID uint, body string) *models.Comment {
	t.Helper()
	c := models.Comment{
		UserID:    userID,
		MatchupID: matchupID,
		Body:      body,
	}
	c.Prepare()
	saved, err := c.SaveComment(db)
	if err != nil {
		t.Fatalf("seed comment: %v", err)
	}
	return saved
}

// authedCtx creates a context with the user ID and optionally admin flag set.
func authedCtx(userID uint, isAdmin bool) context.Context {
	ctx := context.Background()
	ctx = httpctx.WithUserID(ctx, userID)
	if isAdmin {
		ctx = httpctx.WithIsAdmin(ctx, true)
	}
	return ctx
}

// connectReq builds a connect.Request with the given message and optional auth header.
func connectReq[T any](msg *T, token string) *connect.Request[T] {
	req := connect.NewRequest(msg)
	if token != "" {
		req.Header().Set("Authorization", "Bearer "+token)
	}
	return req
}

// connectReqWithCtx builds a connect.Request and injects userID into the
// underlying HTTP request context. This simulates what the JWT middleware does.
func connectReqWithCtx[T any](msg *T, userID uint, isAdmin bool) *connect.Request[T] {
	ctx := authedCtx(userID, isAdmin)
	httpReq := httptest.NewRequest("POST", "/", nil).WithContext(ctx)
	req := connect.NewRequest(msg)
	// Copy headers — the context lives on the handler's ctx parameter,
	// which ConnectRPC populates from the interceptor chain. For unit-style
	// integration tests we call the handler method directly, so we just
	// pass authedCtx as the ctx argument.
	_ = httpReq
	return req
}

// assertConnectError checks that an error is a connect.Error with the expected code.
func assertConnectError(t *testing.T, err error, expectedCode connect.Code) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var connectErr *connect.Error
	if ok := isConnectError(err, &connectErr); !ok {
		t.Fatalf("expected connect.Error, got %T: %v", err, err)
	}
	if connectErr.Code() != expectedCode {
		t.Errorf("expected code %v, got %v: %s", expectedCode, connectErr.Code(), connectErr.Message())
	}
}

func isConnectError(err error, target **connect.Error) bool {
	for err != nil {
		if ce, ok := err.(*connect.Error); ok {
			*target = ce
			return true
		}
		// unwrap
		type unwrapper interface{ Unwrap() error }
		if u, ok := err.(unwrapper); ok {
			err = u.Unwrap()
		} else {
			return false
		}
	}
	return false
}

// refreshSnapshotsDirect refreshes every materialized view that backs the
// home feed or popular endpoints. Integration tests that exercise MV-backed
// paths should seed data first, then call this to make that data visible.
//
// Uses non-CONCURRENTLY refresh because it's simpler and faster on the tiny
// per-test databases we create — the CONCURRENT form exists to avoid blocking
// readers in production, which is not a concern in an isolated test DB.
func refreshSnapshotsDirect(t *testing.T, db *sqlx.DB) {
	t.Helper()
	views := []string{
		"popular_matchups_snapshot",
		"popular_brackets_snapshot",
		"home_summary_snapshot",
		"home_creators_snapshot",
		"home_new_this_week_snapshot",
		"trending_matchups_snapshot",
		"most_played_snapshot",
	}
	for _, v := range views {
		if _, err := db.Exec("REFRESH MATERIALIZED VIEW public." + v); err != nil {
			t.Fatalf("refresh %s: %v", v, err)
		}
	}
}

// ensureNoRow is a helper that checks a query returns sql.ErrNoRows.
func ensureNoRow(t *testing.T, db *sqlx.DB, query string, args ...interface{}) {
	t.Helper()
	var dummy int
	err := db.Get(&dummy, query, args...)
	if err == nil {
		t.Errorf("expected no row for query %q, but found one", query)
	} else if err != sql.ErrNoRows {
		t.Errorf("unexpected error for query %q: %v", query, err)
	}
}
