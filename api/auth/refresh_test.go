//go:build integration

package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

// Refresh-token tests talk to a real Postgres because the reuse
// detection + rotation atomicity are the whole value of this code —
// mocking the DB would just be testing my mocks. Behind the
// `integration` build tag so `go test ./...` without the tag still
// runs clean.

func setupAuthTestDB(t *testing.T) *sqlx.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		host := envOr("TEST_DB_HOST", "127.0.0.1")
		user := envOr("TEST_DB_USER", os.Getenv("DB_USER"))
		pass := envOr("TEST_DB_PASSWORD", os.Getenv("DB_PASSWORD"))
		port := envOr("TEST_DB_PORT", "5432")
		dsn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable", host, port, user, pass)
	}

	admin, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Skipf("postgres not reachable (%v); skipping refresh-token integration tests", err)
	}

	dbName := fmt.Sprintf("matchup_auth_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE " + dbName); err != nil {
		t.Fatalf("create test db: %v", err)
	}

	testDSN := strings.Replace(dsn, "dbname=postgres", "dbname="+dbName, 1)
	db, err := sqlx.Connect("postgres", testDSN)
	if err != nil {
		admin.Exec("DROP DATABASE " + dbName)
		t.Fatalf("connect test db: %v", err)
	}

	if err := goose.Up(db.DB, "../migrations"); err != nil {
		db.Close()
		admin.Exec("DROP DATABASE " + dbName)
		t.Fatalf("goose migrations: %v", err)
	}

	// Seed one user so FK constraints on refresh_tokens.user_id pass.
	if _, err := db.Exec(`
		INSERT INTO users (public_id, username, email, password, avatar_path, is_admin, is_private, followers_count, following_count, created_at, updated_at)
		VALUES ('00000000-0000-0000-0000-000000000001', 'alice', 'alice@example.com', 'hash', '', false, false, 0, 0, NOW(), NOW())`,
	); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		admin.Exec("DROP DATABASE " + dbName + " WITH (FORCE)")
		admin.Close()
	})
	return db
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func firstUserID(t *testing.T, db *sqlx.DB) uint {
	t.Helper()
	var id uint
	if err := db.Get(&id, "SELECT id FROM users ORDER BY id ASC LIMIT 1"); err != nil {
		t.Fatalf("fetch user id: %v", err)
	}
	return id
}

// TestMint_InsertsRowWithNewFamily — first token for a user gets a
// fresh family_id, NULL used_at, NULL revoked_at.
func TestMint_InsertsRowWithNewFamily(t *testing.T) {
	db := setupAuthTestDB(t)
	uid := firstUserID(t, db)

	plaintext, err := MintRefreshToken(context.Background(), db, uid, "test-ua")
	if err != nil {
		t.Fatalf("MintRefreshToken: %v", err)
	}
	if plaintext == "" {
		t.Fatal("expected non-empty plaintext")
	}

	var row struct {
		UserID    uint       `db:"user_id"`
		UsedAt    *time.Time `db:"used_at"`
		RevokedAt *time.Time `db:"revoked_at"`
		UserAgent *string    `db:"user_agent"`
	}
	if err := db.Get(&row,
		"SELECT user_id, used_at, revoked_at, user_agent FROM refresh_tokens WHERE token_hash = $1",
		hashRefreshToken(plaintext),
	); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if row.UserID != uid {
		t.Errorf("want user_id=%d, got %d", uid, row.UserID)
	}
	if row.UsedAt != nil || row.RevokedAt != nil {
		t.Errorf("fresh token should have NULL used_at + revoked_at; got used=%v revoked=%v", row.UsedAt, row.RevokedAt)
	}
	if row.UserAgent == nil || *row.UserAgent != "test-ua" {
		t.Errorf("want user_agent=test-ua, got %v", row.UserAgent)
	}
}

// TestRotate_Happy — present a valid token, get back a new plaintext;
// the old row is stamped used_at, the new row exists in the same
// family with NULL used_at.
func TestRotate_Happy(t *testing.T) {
	db := setupAuthTestDB(t)
	uid := firstUserID(t, db)

	original, err := MintRefreshToken(context.Background(), db, uid, "ua1")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	rotated, gotUID, err := RotateRefreshToken(context.Background(), db, original, "ua2")
	if err != nil {
		t.Fatalf("RotateRefreshToken: %v", err)
	}
	if rotated == original {
		t.Fatal("rotation must return a new plaintext")
	}
	if gotUID != uid {
		t.Errorf("returned uid %d != original %d", gotUID, uid)
	}

	// Original row: used_at stamped.
	var origUsed *time.Time
	_ = db.Get(&origUsed, "SELECT used_at FROM refresh_tokens WHERE token_hash = $1", hashRefreshToken(original))
	if origUsed == nil {
		t.Error("original row should have used_at stamped")
	}

	// Rotated row: same family_id, NULL used_at, FK to original.
	var rotatedRow struct {
		FamilyID   string     `db:"family_id"`
		UsedAt     *time.Time `db:"used_at"`
		PreviousID *int64     `db:"previous_id"`
	}
	_ = db.Get(&rotatedRow,
		"SELECT family_id, used_at, previous_id FROM refresh_tokens WHERE token_hash = $1",
		hashRefreshToken(rotated),
	)
	if rotatedRow.UsedAt != nil {
		t.Error("rotated row used_at should be NULL")
	}
	if rotatedRow.PreviousID == nil {
		t.Error("rotated row should link back via previous_id")
	}

	// Family check — both rows share a family_id.
	var origFamilyID string
	_ = db.Get(&origFamilyID, "SELECT family_id FROM refresh_tokens WHERE token_hash = $1", hashRefreshToken(original))
	if origFamilyID != rotatedRow.FamilyID {
		t.Errorf("family_id should be shared: orig=%s rotated=%s", origFamilyID, rotatedRow.FamilyID)
	}
}

// TestRotate_ReuseRevokesFamily — reusing an already-rotated token
// marks every row in the family revoked_at and returns ErrTokenReuse.
// Covers the theft-detection contract.
func TestRotate_ReuseRevokesFamily(t *testing.T) {
	db := setupAuthTestDB(t)
	uid := firstUserID(t, db)

	original, _ := MintRefreshToken(context.Background(), db, uid, "")
	rotated, _, err := RotateRefreshToken(context.Background(), db, original, "")
	if err != nil {
		t.Fatalf("first rotate: %v", err)
	}

	// Rotate ONCE MORE using the original plaintext — simulates the
	// attacker having stolen the pre-rotation token.
	_, _, err = RotateRefreshToken(context.Background(), db, original, "")
	if !errors.Is(err, ErrTokenReuse) {
		t.Fatalf("expected ErrTokenReuse on reuse, got %v", err)
	}

	// Every row in the family should now have revoked_at set —
	// both the original and the (innocent-but-tainted) rotated child.
	var activeCount int
	_ = db.Get(&activeCount,
		"SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1 AND revoked_at IS NULL",
		uid,
	)
	if activeCount != 0 {
		t.Errorf("expected 0 active rows after family revoke, got %d", activeCount)
	}

	// And the rotated child token also no longer rotates — the
	// legitimate user is forced to re-login.
	_, _, err = RotateRefreshToken(context.Background(), db, rotated, "")
	if !errors.Is(err, ErrTokenRevoked) {
		t.Errorf("expected rotated child to be revoked, got %v", err)
	}
}

// TestRotate_Expired — stale row returns ErrTokenExpired without
// creating a rotated child.
func TestRotate_Expired(t *testing.T) {
	db := setupAuthTestDB(t)
	uid := firstUserID(t, db)

	plaintext, _ := MintRefreshToken(context.Background(), db, uid, "")
	// Hand-roll the row into the past.
	if _, err := db.Exec(
		"UPDATE refresh_tokens SET expires_at = NOW() - INTERVAL '1 second' WHERE token_hash = $1",
		hashRefreshToken(plaintext),
	); err != nil {
		t.Fatalf("age token: %v", err)
	}

	_, _, err := RotateRefreshToken(context.Background(), db, plaintext, "")
	if !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}

	// No rotated child got inserted.
	var total int
	_ = db.Get(&total, "SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1", uid)
	if total != 1 {
		t.Errorf("expected exactly 1 row after failed rotation, got %d", total)
	}
}

// TestRotate_NotFound — nonsense plaintext returns ErrTokenNotFound.
func TestRotate_NotFound(t *testing.T) {
	db := setupAuthTestDB(t)

	_, _, err := RotateRefreshToken(context.Background(), db, "totally-fake-token", "")
	if !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("expected ErrTokenNotFound, got %v", err)
	}
}

// TestRevokeRefreshToken_SingleRow — stamps revoked_at on the
// specified row; unrelated rows untouched.
func TestRevokeRefreshToken_SingleRow(t *testing.T) {
	db := setupAuthTestDB(t)
	uid := firstUserID(t, db)

	keepMe, _ := MintRefreshToken(context.Background(), db, uid, "")
	killMe, _ := MintRefreshToken(context.Background(), db, uid, "")

	if err := RevokeRefreshToken(context.Background(), db, killMe); err != nil {
		t.Fatalf("RevokeRefreshToken: %v", err)
	}

	var keepRevoked, killRevoked *time.Time
	_ = db.Get(&keepRevoked, "SELECT revoked_at FROM refresh_tokens WHERE token_hash = $1", hashRefreshToken(keepMe))
	_ = db.Get(&killRevoked, "SELECT revoked_at FROM refresh_tokens WHERE token_hash = $1", hashRefreshToken(killMe))
	if keepRevoked != nil {
		t.Error("keepMe shouldn't be revoked")
	}
	if killRevoked == nil {
		t.Error("killMe should be revoked")
	}
}

// TestRevokeAllUserTokens — stamps revoked_at on every non-revoked
// row for the user; returns the count.
func TestRevokeAllUserTokens(t *testing.T) {
	db := setupAuthTestDB(t)
	uid := firstUserID(t, db)

	// Three active rows for alice.
	for i := 0; i < 3; i++ {
		if _, err := MintRefreshToken(context.Background(), db, uid, ""); err != nil {
			t.Fatalf("mint %d: %v", i, err)
		}
	}

	n, err := RevokeAllUserTokens(context.Background(), db, uid)
	if err != nil {
		t.Fatalf("RevokeAllUserTokens: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3 revoked, got %d", n)
	}

	// A follow-up call revokes zero (idempotency check).
	n2, _ := RevokeAllUserTokens(context.Background(), db, uid)
	if n2 != 0 {
		t.Errorf("second call should revoke 0, got %d", n2)
	}
}

// TestRevokeRefreshToken_EmptyPlaintext — a nil/empty string is a
// no-op rather than a SQL error.
func TestRevokeRefreshToken_EmptyPlaintext(t *testing.T) {
	db := setupAuthTestDB(t)
	if err := RevokeRefreshToken(context.Background(), db, ""); err != nil {
		t.Errorf("expected nil for empty plaintext, got %v", err)
	}
}
