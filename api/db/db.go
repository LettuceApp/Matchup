package db

import (
	"context"
	"os"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/twinj/uuid"
)

// Psql is a Squirrel statement builder configured for PostgreSQL ($1 placeholders).
var Psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

// Connect opens a sqlx connection to the primary (read/write) PostgreSQL
// instance and applies the standard connection-pool tuning.
func Connect(dsn string) (*sqlx.DB, error) {
	return openWithPool(dsn)
}

// ConnectRead opens a sqlx connection to a read-only PostgreSQL replica.
// Identical pool tuning as Connect — the only reason it's a separate function
// is so call sites can be self-documenting about which physical instance the
// pool is attached to. See Step 19 of the scalability plan and
// `controllers.Server.ReadDB` for how reads are routed.
func ConnectRead(dsn string) (*sqlx.DB, error) {
	return openWithPool(dsn)
}

func openWithPool(dsn string) (*sqlx.DB, error) {
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}

// GeneratePublicID returns a new V4 UUID string.
func GeneratePublicID() string {
	return uuid.NewV4().String()
}

// ProcessAvatarPath converts a relative avatar path to a full S3 URL.
// If the path is empty or already a full URL, it is returned as-is.
//
// Equivalent to ProcessAvatarPathSized(path, "") — kept for callers that
// only ever want the canonical full-size URL.
func ProcessAvatarPath(path string) string {
	return ProcessAvatarPathSized(path, "")
}

// ProcessAvatarPathSized is the size-aware variant. Pass "thumb" or "medium"
// to get the resized JPEG variant uploaded by resizeAndUpload (controllers
// package). Empty string returns the canonical full-size URL. Sizes that
// aren't part of the upload ladder still produce a URL, but it will 404 in S3.
func ProcessAvatarPathSized(path, size string) string {
	return processImagePathSized(path, "UserProfilePics/", size)
}

// ProcessMatchupImagePath converts a relative matchup image path to a full S3 URL.
func ProcessMatchupImagePath(path string) string {
	return ProcessMatchupImagePathSized(path, "")
}

// ProcessMatchupImagePathSized is the size-aware variant. See
// ProcessAvatarPathSized for the size argument semantics.
func ProcessMatchupImagePathSized(path, size string) string {
	return processImagePathSized(path, "MatchupImages/", size)
}

// processImagePathSized is the shared implementation for both avatar and
// matchup-image S3 URL construction. dirPrefix is the S3 key directory
// ("UserProfilePics/" or "MatchupImages/"). We insert the "_<size>"
// suffix into the filename portion of the key (before the extension) so
// the result matches what resizeAndUpload writes. For unrecognised paths
// (no extension, or extension not at the end of a filename) we fall
// back to the unsuffixed key — better to serve the original than a 404.
func processImagePathSized(path, dirPrefix, size string) string {
	if path == "" || strings.HasPrefix(path, "http") {
		return path
	}
	bucket := os.Getenv("S3_BUCKET")
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-2"
	}
	key := path
	if !strings.HasPrefix(key, dirPrefix) {
		key = dirPrefix + key
	}
	if size != "" {
		key = insertSizeSuffix(key, "_"+size)
	}
	return "https://" + bucket + ".s3." + region + ".amazonaws.com/" + key
}

// insertSizeSuffix inserts suffix (e.g. "_thumb") into a key just before the
// final extension. "UserProfilePics/avatar-abc.jpg" + "_thumb" becomes
// "UserProfilePics/avatar-abc_thumb.jpg". If the key has no extension at all,
// the suffix is appended to the end. The dot must come AFTER the last "/"
// for it to count as an extension — this avoids splitting on dots in
// directory names.
func insertSizeSuffix(key, suffix string) string {
	dot := strings.LastIndex(key, ".")
	if dot < 0 || dot < strings.LastIndex(key, "/") {
		return key + suffix
	}
	return key[:dot] + suffix + key[dot:]
}

// ProcessUserAvatars processes avatar paths for a slice of users (or any struct
// with an AvatarPath field). Call after scanning rows.
func ProcessUserAvatarPaths(avatarPath *string) {
	if avatarPath != nil {
		*avatarPath = ProcessAvatarPath(*avatarPath)
	}
}

// DBTX is the interface satisfied by both *sqlx.DB and *sqlx.Tx.
// Model methods should accept this so they work inside or outside transactions.
type DBTX interface {
	sqlx.ExtContext
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
}
