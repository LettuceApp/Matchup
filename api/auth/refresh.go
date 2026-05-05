package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// RefreshTokenTTL is the rolling window for a refresh token. Each
// successful rotation inserts a fresh row whose expires_at starts
// from NOW() — so a user who signs in once and keeps using the app
// never has to re-enter credentials, but a completely idle session
// ages out after 30 days of silence. Exposed as var only for tests.
var RefreshTokenTTL = 30 * 24 * time.Hour

// Sentinel errors returned to handlers. The Connect RPC layer maps
// all of these to `CodeUnauthenticated` with a generic
// "invalid refresh token" message so attackers can't probe to learn
// which state a token is in.
var (
	ErrTokenNotFound = errors.New("refresh: token not found")
	ErrTokenExpired  = errors.New("refresh: token expired")
	ErrTokenReuse    = errors.New("refresh: token already used (family revoked)")
	ErrTokenRevoked  = errors.New("refresh: token revoked")
)

// MintRefreshToken inserts a brand-new refresh-token row with a new
// family_id and returns the plaintext for the caller to ship back in
// the RPC response. Use at Login time.
//
// userAgent is optional debugging metadata; pass "" if unknown.
func MintRefreshToken(ctx context.Context, db *sqlx.DB, userID uint, userAgent string) (string, error) {
	plaintext, err := generatePlaintextRefreshToken()
	if err != nil {
		return "", err
	}
	familyID := uuid.New()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO public.refresh_tokens
			(user_id, token_hash, family_id, user_agent, expires_at)
		VALUES ($1, $2, $3, $4, $5)`,
		userID,
		hashRefreshToken(plaintext),
		familyID,
		nullableUA(userAgent),
		time.Now().Add(RefreshTokenTTL),
	); err != nil {
		return "", err
	}
	return plaintext, nil
}

// refreshTokenRow mirrors what we read back from the table when
// rotating. Kept package-private — callers only need the errors + the
// new plaintext.
type refreshTokenRow struct {
	ID         int64      `db:"id"`
	UserID     uint       `db:"user_id"`
	FamilyID   uuid.UUID  `db:"family_id"`
	ExpiresAt  time.Time  `db:"expires_at"`
	UsedAt     *time.Time `db:"used_at"`
	RevokedAt  *time.Time `db:"revoked_at"`
}

// RotateRefreshToken is the heart of the flow. It:
//  1. looks up the row by token_hash;
//  2. applies the guard ladder (expired / revoked / already-used);
//  3. on "already-used" → REVOKES the whole family (theft detection);
//  4. on success → stamps used_at on the presented row AND inserts a
//     new row in the same family, returning the new plaintext.
//
// Done in a single transaction so a concurrent refresh with the same
// plaintext can't both stamp used_at AND both insert a child —
// SERIALIZABLE isolation wins the race for exactly one caller; the
// other sees its own UPDATE affect zero rows and bails with
// ErrTokenReuse (which is correct: only one of them should have had
// this token legitimately).
func RotateRefreshToken(ctx context.Context, db *sqlx.DB, plaintext, userAgent string) (newPlaintext string, userID uint, err error) {
	tx, err := db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return "", 0, err
	}
	// Rollback is a no-op after Commit; blanket defer is safe.
	defer tx.Rollback()

	hash := hashRefreshToken(plaintext)

	var row refreshTokenRow
	err = tx.GetContext(ctx, &row,
		`SELECT id, user_id, family_id, expires_at, used_at, revoked_at
		   FROM public.refresh_tokens
		  WHERE token_hash = $1`, hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, ErrTokenNotFound
	}
	if err != nil {
		return "", 0, err
	}

	// Guard ladder — order matters. Reuse check beats expiry/revoke so
	// a stolen-but-expired token still trips the theft signal.
	if row.UsedAt != nil {
		// Theft detected — someone presented a token that's already
		// been rotated away. Revoke the whole family, including any
		// currently-valid descendant the legitimate user is holding.
		// They'll be forced to re-login on their next request; the
		// attacker is equally locked out.
		if _, err := tx.ExecContext(ctx, `
			UPDATE public.refresh_tokens
			   SET revoked_at = NOW()
			 WHERE family_id = $1 AND revoked_at IS NULL`,
			row.FamilyID,
		); err != nil {
			return "", 0, err
		}
		if err := tx.Commit(); err != nil {
			return "", 0, err
		}
		return "", 0, ErrTokenReuse
	}
	if row.RevokedAt != nil {
		return "", 0, ErrTokenRevoked
	}
	if time.Now().After(row.ExpiresAt) {
		return "", 0, ErrTokenExpired
	}

	// Happy path: stamp used_at on the presented row, insert the
	// rotated child in the same family.
	if _, err := tx.ExecContext(ctx,
		`UPDATE public.refresh_tokens SET used_at = NOW() WHERE id = $1`,
		row.ID,
	); err != nil {
		return "", 0, err
	}

	newPlaintext, err = generatePlaintextRefreshToken()
	if err != nil {
		return "", 0, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO public.refresh_tokens
			(user_id, token_hash, family_id, previous_id, user_agent, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		row.UserID,
		hashRefreshToken(newPlaintext),
		row.FamilyID,
		row.ID,
		nullableUA(userAgent),
		time.Now().Add(RefreshTokenTTL),
	); err != nil {
		return "", 0, err
	}

	if err := tx.Commit(); err != nil {
		return "", 0, err
	}
	return newPlaintext, row.UserID, nil
}

// RevokeRefreshToken stamps revoked_at on the single row matching the
// plaintext. Idempotent: absent / already-revoked rows are silently
// accepted (Logout should never surface "nothing to do" to the user,
// and hiding the distinction foils "which of these tokens is valid?"
// probing).
func RevokeRefreshToken(ctx context.Context, db *sqlx.DB, plaintext string) error {
	if plaintext == "" {
		return nil
	}
	_, err := db.ExecContext(ctx, `
		UPDATE public.refresh_tokens
		   SET revoked_at = NOW()
		 WHERE token_hash = $1
		   AND revoked_at IS NULL`,
		hashRefreshToken(plaintext),
	)
	return err
}

// RevokeAllUserTokens revokes every non-revoked row for the user.
// Used by LogoutAll and (in a later cycle) on password change.
// Returns the number of rows freshly revoked.
func RevokeAllUserTokens(ctx context.Context, db *sqlx.DB, userID uint) (int, error) {
	result, err := db.ExecContext(ctx, `
		UPDATE public.refresh_tokens
		   SET revoked_at = NOW()
		 WHERE user_id = $1
		   AND revoked_at IS NULL`,
		userID,
	)
	if err != nil {
		return 0, err
	}
	n, err := result.RowsAffected()
	return int(n), err
}

// hashRefreshToken is SHA-256 hex. No salt (the plaintext is already
// 256 bits of crypto/rand entropy — a salted slow hash would only
// burn CPU on the refresh hot path without adding meaningful security).
func hashRefreshToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// generatePlaintextRefreshToken: 32 bytes from crypto/rand, base64url
// with no padding. Matches the "opaque random bearer token" shape
// every major OAuth library uses.
func generatePlaintextRefreshToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func nullableUA(ua string) interface{} {
	if ua == "" {
		return nil
	}
	return ua
}
