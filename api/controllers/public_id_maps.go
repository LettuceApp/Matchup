package controllers

import (
	"Matchup/models"

	"gorm.io/gorm"
)

type idPublicPair struct {
	ID       uint
	PublicID string
}

func loadUserPublicIDMap(db *gorm.DB, ids []uint) map[uint]string {
	if len(ids) == 0 {
		return map[uint]string{}
	}

	var rows []idPublicPair
	if err := db.Model(&models.User{}).
		Select("id", "public_id").
		Where("id IN ?", ids).
		Scan(&rows).Error; err != nil {
		return map[uint]string{}
	}

	result := make(map[uint]string, len(rows))
	for _, row := range rows {
		result[row.ID] = row.PublicID
	}
	return result
}

func loadMatchupPublicIDMap(db *gorm.DB, ids []uint) map[uint]string {
	if len(ids) == 0 {
		return map[uint]string{}
	}

	var rows []idPublicPair
	if err := db.Model(&models.Matchup{}).
		Select("id", "public_id").
		Where("id IN ?", ids).
		Scan(&rows).Error; err != nil {
		return map[uint]string{}
	}

	result := make(map[uint]string, len(rows))
	for _, row := range rows {
		result[row.ID] = row.PublicID
	}
	return result
}

func loadBracketPublicIDMap(db *gorm.DB, ids []uint) map[uint]string {
	if len(ids) == 0 {
		return map[uint]string{}
	}

	var rows []idPublicPair
	if err := db.Model(&models.Bracket{}).
		Select("id", "public_id").
		Where("id IN ?", ids).
		Scan(&rows).Error; err != nil {
		return map[uint]string{}
	}

	result := make(map[uint]string, len(rows))
	for _, row := range rows {
		result[row.ID] = row.PublicID
	}
	return result
}

func loadMatchupItemPublicIDMap(db *gorm.DB, ids []uint) map[uint]string {
	if len(ids) == 0 {
		return map[uint]string{}
	}

	var rows []idPublicPair
	if err := db.Model(&models.MatchupItem{}).
		Select("id", "public_id").
		Where("id IN ?", ids).
		Scan(&rows).Error; err != nil {
		return map[uint]string{}
	}

	result := make(map[uint]string, len(rows))
	for _, row := range rows {
		result[row.ID] = row.PublicID
	}
	return result
}
