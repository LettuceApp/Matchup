package controllers

import (
	"errors"

	"Matchup/models"

	"gorm.io/gorm"
)

func mergeDeviceVotesToUser(db *gorm.DB, userID uint, deviceID string) error {
	if deviceID == "" {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var votes []models.MatchupVote
		if err := tx.Where("anon_id = ?", deviceID).Find(&votes).Error; err != nil {
			return err
		}

		for _, vote := range votes {
			var existing models.MatchupVote
			err := tx.Where("user_id = ? AND matchup_public_id = ?", userID, vote.MatchupPublicID).First(&existing).Error
			if err == nil {
				var previousItem models.MatchupItem
				if err := tx.Where("public_id = ?", vote.MatchupItemPublicID).
					First(&previousItem).Error; err != nil {
					return err
				}
				if err := tx.Model(&models.MatchupItem{}).
					Where("id = ?", previousItem.ID).
					UpdateColumn("votes", gorm.Expr("CASE WHEN votes > 0 THEN votes - 1 ELSE 0 END")).
					Error; err != nil {
					return err
				}
				if err := tx.Delete(&models.MatchupVote{}, vote.ID).Error; err != nil {
					return err
				}
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}

			if err := tx.Model(&models.MatchupVote{}).
				Where("id = ?", vote.ID).
				Updates(map[string]interface{}{
					"user_id":   userID,
					"anon_id":   nil,
				}).Error; err != nil {
				return err
			}
		}

		return nil
	})
}
