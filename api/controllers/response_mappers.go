package controllers

import (
	"context"
	"strconv"
	"time"

	"Matchup/models"

	"github.com/jmoiron/sqlx"
)

func uintToString(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}

func timePtrOrNil(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	copy := *t
	return &copy
}

// resolveUserPublicID returns the user's public_id. It uses the pre-loaded user
// object when available to avoid a DB round-trip.
func resolveUserPublicID(db sqlx.ExtContext, user *models.User, userID uint) string {
	if user != nil && user.PublicID != "" {
		return user.PublicID
	}
	if db == nil || userID == 0 {
		return ""
	}
	var publicID string
	if err := sqlx.GetContext(context.Background(), db, &publicID, "SELECT public_id FROM users WHERE id = $1", userID); err != nil {
		return ""
	}
	return publicID
}

func resolveBracketPublicID(db sqlx.ExtContext, bracketID *uint) *string {
	if db == nil || bracketID == nil || *bracketID == 0 {
		return nil
	}
	var publicID string
	if err := sqlx.GetContext(context.Background(), db, &publicID, "SELECT public_id FROM brackets WHERE id = $1", *bracketID); err != nil {
		return nil
	}
	if publicID == "" {
		return nil
	}
	return &publicID
}

func resolveMatchupItemPublicID(db sqlx.ExtContext, itemID uint) string {
	if db == nil || itemID == 0 {
		return ""
	}
	var publicID string
	if err := sqlx.GetContext(context.Background(), db, &publicID, "SELECT public_id FROM matchup_items WHERE id = $1", itemID); err != nil {
		return ""
	}
	return publicID
}
