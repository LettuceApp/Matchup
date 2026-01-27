package controllers

import (
	"errors"
	"strconv"
	"strings"

	"Matchup/models"

	"gorm.io/gorm"
)

var errInvalidIdentifier = errors.New("invalid identifier")

func resolveMatchupByIdentifier(db *gorm.DB, identifier string) (*models.Matchup, error) {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" {
		return nil, errInvalidIdentifier
	}
	var matchup models.Matchup
	if err := db.Where("public_id::text = ?", trimmed).First(&matchup).Error; err == nil {
		return &matchup, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if numericID, err := strconv.ParseUint(trimmed, 10, 32); err == nil {
		if err := db.First(&matchup, uint(numericID)).Error; err != nil {
			return nil, err
		}
		return &matchup, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func resolveBracketByIdentifier(db *gorm.DB, identifier string) (*models.Bracket, error) {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" {
		return nil, errInvalidIdentifier
	}
	var bracket models.Bracket
	if err := db.Where("public_id::text = ?", trimmed).First(&bracket).Error; err == nil {
		return &bracket, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if numericID, err := strconv.ParseUint(trimmed, 10, 32); err == nil {
		if err := db.First(&bracket, uint(numericID)).Error; err != nil {
			return nil, err
		}
		return &bracket, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func resolveMatchupItemByIdentifier(db *gorm.DB, identifier string) (*models.MatchupItem, error) {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" {
		return nil, errInvalidIdentifier
	}
	var item models.MatchupItem
	if err := db.Where("public_id::text = ?", trimmed).First(&item).Error; err == nil {
		return &item, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if numericID, err := strconv.ParseUint(trimmed, 10, 32); err == nil {
		if err := db.First(&item, uint(numericID)).Error; err != nil {
			return nil, err
		}
		return &item, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func resolveCommentByIdentifier(db *gorm.DB, identifier string) (*models.Comment, error) {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" {
		return nil, errInvalidIdentifier
	}
	var comment models.Comment
	if err := db.Where("public_id::text = ?", trimmed).First(&comment).Error; err == nil {
		return &comment, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if numericID, err := strconv.ParseUint(trimmed, 10, 32); err == nil {
		if err := db.First(&comment, uint(numericID)).Error; err != nil {
			return nil, err
		}
		return &comment, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func resolveBracketCommentByIdentifier(db *gorm.DB, identifier string) (*models.BracketComment, error) {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" {
		return nil, errInvalidIdentifier
	}
	var comment models.BracketComment
	if err := db.Where("public_id::text = ?", trimmed).First(&comment).Error; err == nil {
		return &comment, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if numericID, err := strconv.ParseUint(trimmed, 10, 32); err == nil {
		if err := db.First(&comment, uint(numericID)).Error; err != nil {
			return nil, err
		}
		return &comment, nil
	}
	return nil, gorm.ErrRecordNotFound
}
