package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	appdb "Matchup/db"
	"Matchup/models"

	"github.com/jmoiron/sqlx"
)

const anonymousDeviceCookieName = "matchup_device_id"

var errInvalidAuthToken = errors.New("invalid auth token")

type voteIdentity struct {
	UserID *uint
	AnonID *string
}

func upsertAnonymousDevice(db sqlx.ExtContext, deviceID, userAgentHash, ipHash string) error {
	if deviceID == "" {
		return errors.New("device_id required")
	}
	now := time.Now()

	result, err := db.ExecContext(context.Background(),
		"UPDATE anonymous_devices SET last_seen_at = $1, user_agent_hash = $2, ip_hash = $3 WHERE device_id = $4",
		now, userAgentHash, ipHash, deviceID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows > 0 {
		return nil
	}

	_, err = db.ExecContext(context.Background(),
		`INSERT INTO anonymous_devices (device_id, user_agent_hash, ip_hash, created_at, last_seen_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (device_id) DO UPDATE SET last_seen_at = $5, user_agent_hash = $2, ip_hash = $3`,
		deviceID, userAgentHash, ipHash, now, now)
	return err
}

func hashFingerprint(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	salt := os.Getenv("ANON_DEVICE_SALT")
	hash := sha256.Sum256([]byte(salt + ":" + trimmed))
	return hex.EncodeToString(hash[:])
}

func setDeviceCookie(w http.ResponseWriter, deviceID string) {
	secure := os.Getenv("APP_ENV") == "production"
	sameSite := http.SameSiteLaxMode
	if secure {
		sameSite = http.SameSiteNoneMode
	}
	http.SetCookie(w, &http.Cookie{
		Name:     anonymousDeviceCookieName,
		Value:    deviceID,
		Path:     "/",
		MaxAge:   31536000,
		HttpOnly: true,
		SameSite: sameSite,
		Secure:   secure,
	})
}

func isLikelyUUID(value string) bool {
	return len(strings.TrimSpace(value)) >= 32
}

// getOrCreateDeviceIDFromRequest resolves or creates a device ID from an HTTP request.
func getOrCreateDeviceIDFromRequest(r *http.Request, w http.ResponseWriter, db sqlx.ExtContext) (string, error) {
	uaHash := hashFingerprint(r.UserAgent())
	ipHash := hashFingerprint(r.RemoteAddr)

	deviceID := ""
	if cookie, err := r.Cookie(anonymousDeviceCookieName); err == nil && isLikelyUUID(cookie.Value) {
		deviceID = cookie.Value
	} else {
		cutoff := time.Now().Add(-30 * 24 * time.Hour)
		var existing models.AnonymousDevice
		err := sqlx.GetContext(context.Background(), db, &existing,
			"SELECT * FROM anonymous_devices WHERE user_agent_hash = $1 AND ip_hash = $2 AND last_seen_at >= $3 ORDER BY last_seen_at DESC LIMIT 1",
			uaHash, ipHash, cutoff)
		if err == nil {
			deviceID = existing.DeviceID
		}
	}

	if deviceID == "" {
		deviceID = appdb.GeneratePublicID()
	}

	if err := upsertAnonymousDevice(db, deviceID, uaHash, ipHash); err != nil {
		return "", err
	}

	if w != nil {
		setDeviceCookie(w, deviceID)
	}
	return deviceID, nil
}
