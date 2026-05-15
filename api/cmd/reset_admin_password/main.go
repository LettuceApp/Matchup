// reset_admin_password sets the admin user's password by writing a
// fresh bcrypt hash to the users.password column. Useful when the
// admin row exists but has an empty/forgotten password and the
// forgot-password email flow isn't viable (e.g. local dev where
// SendGrid isn't wired up).
//
// Usage:
//
//	go run ./cmd/reset_admin_password
//
// The tool reads ADMIN_EMAIL from the environment (same var
// seedAdmin uses), prompts for the new password twice without
// echoing it to the terminal, generates a bcrypt hash, and writes
// it to the matching row. The plaintext password is never logged,
// stored, or transmitted off the machine.
//
// DB connection re-uses the same DATABASE_URL / DB_* env vars the
// main server reads, so dropping a .env in the working directory is
// enough.
package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"

	"Matchup/security"

	_ "github.com/lib/pq"
	"github.com/joho/godotenv"
	"golang.org/x/term"
)

func main() {
	_ = godotenv.Load()

	email := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL")))
	if email == "" {
		log.Fatal("ADMIN_EMAIL is not set. Add it to your .env or export it before running.")
	}

	dsn, err := buildDSN()
	if err != nil {
		log.Fatalf("could not build DB DSN: %v", err)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("could not open DB: %v", err)
	}
	defer db.Close()

	// Confirm the row exists before asking for input — saves the
	// user from typing a password twice only to learn the email
	// doesn't match anything.
	var (
		id      int64
		isAdmin bool
	)
	err = db.QueryRow(
		"SELECT id, is_admin FROM users WHERE LOWER(email) = $1",
		email,
	).Scan(&id, &isAdmin)
	if errors.Is(err, sql.ErrNoRows) {
		log.Fatalf("no user found with email %q — check ADMIN_EMAIL.", email)
	}
	if err != nil {
		log.Fatalf("could not look up user: %v", err)
	}
	if !isAdmin {
		fmt.Printf("Warning: user %q exists but is_admin=false. Continuing anyway.\n", email)
	}

	pw, err := readSecret(fmt.Sprintf("New password for %s: ", email))
	if err != nil {
		log.Fatalf("could not read password: %v", err)
	}
	if len(pw) < 8 {
		log.Fatal("password must be at least 8 characters.")
	}
	confirm, err := readSecret("Confirm password: ")
	if err != nil {
		log.Fatalf("could not read confirmation: %v", err)
	}
	if pw != confirm {
		log.Fatal("passwords do not match.")
	}

	hash, err := security.Hash(pw)
	if err != nil {
		log.Fatalf("could not hash password: %v", err)
	}

	if _, err := db.Exec(
		`UPDATE users SET password = $1, updated_at = NOW() WHERE id = $2`,
		string(hash), id,
	); err != nil {
		log.Fatalf("could not update password: %v", err)
	}

	fmt.Printf("Password updated for user %q (id=%d). You can now sign in.\n", email, id)
}

// readSecret prompts and reads a line from /dev/tty with echo
// disabled. Falls back to stdin if no TTY is attached (so this
// still works under `go run`, though without the no-echo guarantee).
func readSecret(prompt string) (string, error) {
	fmt.Print(prompt)
	bytes, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bytes)), nil
}

// buildDSN mirrors api/server.go: prefer DATABASE_URL in prod;
// otherwise stitch one together from DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME.
func buildDSN() (string, error) {
	if url := strings.TrimSpace(os.Getenv("DATABASE_URL")); url != "" {
		return url, nil
	}
	host := envOrDefault("DB_HOST", "localhost")
	port := envOrDefault("DB_PORT", "5432")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	name := os.Getenv("DB_NAME")
	if user == "" || name == "" {
		return "", fmt.Errorf("DB_USER and DB_NAME must be set (or supply DATABASE_URL)")
	}
	// sslmode=disable matches the rest of the local dev surface.
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, name,
	), nil
}

func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
