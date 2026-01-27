package controllers

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"Matchup/auth"
	"Matchup/models"

	"github.com/gin-gonic/gin"
	"github.com/twinj/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const anonymousDeviceCookieName = "matchup_device_id"

var errInvalidAuthToken = errors.New("invalid auth token")

type voteIdentity struct {
	UserID *uint
	AnonID *string
}

func resolveVoteIdentity(c *gin.Context, db *gorm.DB) (voteIdentity, error) {
	if token := auth.ExtractToken(c.Request); token != "" {
		uid, err := auth.ExtractTokenID(c.Request)
		if err == nil {
			return voteIdentity{UserID: &uid}, nil
		}
	}

	deviceID, err := getOrCreateDeviceID(c, db)
	if err != nil {
		return voteIdentity{}, err
	}
	return voteIdentity{AnonID: &deviceID}, nil
}

func getOrCreateDeviceID(c *gin.Context, db *gorm.DB) (string, error) {
	uaHash := hashFingerprint(c.Request.UserAgent())
	ipHash := hashFingerprint(c.ClientIP())

	deviceID := ""
	if cookie, err := c.Cookie(anonymousDeviceCookieName); err == nil && isLikelyUUID(cookie) {
		deviceID = cookie
	} else {
		cutoff := time.Now().Add(-30 * 24 * time.Hour)
		var existing models.AnonymousDevice
		if err := db.Where("user_agent_hash = ? AND ip_hash = ? AND last_seen_at >= ?", uaHash, ipHash, cutoff).
			Order("last_seen_at DESC").
			First(&existing).Error; err == nil {
			deviceID = existing.DeviceID
		}
	}

	if deviceID == "" {
		deviceID = uuid.NewV4().String()
	}

	if err := upsertAnonymousDevice(db, deviceID, uaHash, ipHash); err != nil {
		return "", err
	}

	setDeviceCookie(c, deviceID)
	return deviceID, nil
}

func upsertAnonymousDevice(db *gorm.DB, deviceID, userAgentHash, ipHash string) error {
	if deviceID == "" {
		return errors.New("device_id required")
	}
	now := time.Now()
	record := models.AnonymousDevice{
		DeviceID:      deviceID,
		UserAgentHash: userAgentHash,
		IPHash:        ipHash,
		CreatedAt:     now,
		LastSeenAt:    now,
	}

	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "device_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_seen_at", "user_agent_hash", "ip_hash"}),
	}).Create(&record).Error
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

func setDeviceCookie(c *gin.Context, deviceID string) {
	secure := os.Getenv("APP_ENV") == "production"
	sameSite := http.SameSiteLaxMode
	if secure {
		sameSite = http.SameSiteNoneMode
	}
	http.SetCookie(c.Writer, &http.Cookie{
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
