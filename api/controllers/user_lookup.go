package controllers

import (
	"errors"
	"strings"

	"Matchup/models"

	"gorm.io/gorm"
)

func resolveUserByIdentifier(db *gorm.DB, identifier string) (*models.User, error) {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" {
		return nil, gorm.ErrRecordNotFound
	}

	var user models.User
	if isUUIDLike(trimmed) {
		if err := db.Where("public_id::text = ?", trimmed).First(&user).Error; err == nil {
			return &user, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	username := strings.ToLower(trimmed)
	if err := db.Where("username = ?", username).First(&user).Error; err == nil {
		return &user, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if err := db.Where("public_id::text = ?", trimmed).First(&user).Error; err != nil {
		return nil, err
	}

	return &user, nil
}
