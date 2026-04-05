package db

import (
	"context"
	"os"
	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/twinj/uuid"
)

// Psql is a Squirrel statement builder configured for PostgreSQL ($1 placeholders).
var Psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

// Connect opens a sqlx connection to PostgreSQL.
func Connect(dsn string) (*sqlx.DB, error) {
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// GeneratePublicID returns a new V4 UUID string.
func GeneratePublicID() string {
	return uuid.NewV4().String()
}

// ProcessAvatarPath converts a relative avatar path to a full S3 URL.
// If the path is empty or already a full URL, it is returned as-is.
func ProcessAvatarPath(path string) string {
	if path == "" || strings.HasPrefix(path, "http") {
		return path
	}
	bucket := os.Getenv("S3_BUCKET")
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-2"
	}
	key := path
	if !strings.HasPrefix(key, "UserProfilePics/") {
		key = "UserProfilePics/" + key
	}
	return "https://" + bucket + ".s3." + region + ".amazonaws.com/" + key
}

// ProcessMatchupImagePath converts a relative matchup image path to a full S3 URL.
func ProcessMatchupImagePath(path string) string {
	if path == "" || strings.HasPrefix(path, "http") {
		return path
	}
	bucket := os.Getenv("S3_BUCKET")
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-2"
	}
	key := path
	if !strings.HasPrefix(key, "MatchupImages/") {
		key = "MatchupImages/" + key
	}
	return "https://" + bucket + ".s3." + region + ".amazonaws.com/" + key
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
