package models

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Like struct {
	ID        uuid.UUID `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uuid.UUID `gorm:"not null" json:"user_id"`
	MatchupID uuid.UUID `gorm:"not null" json:"matchup_id"`
	User      User      `json:"user"`
	CreatedAt time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (l *Like) SaveLike(db *gorm.DB) (*Like, error) {

	// Check if the auth user has liked this matchup before:
	err := db.Where("matchup_id = ? AND user_id = ?", l.MatchupID, l.UserID).First(&l).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// The user has not liked this matchup before, so lets save incoming like:
			err = db.Create(&l).Error
			if err != nil {
				return &Like{}, err
			}
		}
	} else {
		// The user has liked it before, so create a custom error message
		err = errors.New("double like")
		return &Like{}, err
	}
	return l, nil
}

func (l *Like) DeleteLike(db *gorm.DB) (*Like, error) {
	var err error
	var deletedLike *Like

	err = db.Where("id = ?", l.ID).First(&l).Error
	if err != nil {
		return &Like{}, err
	} else {
		//If the like exist, save it in deleted like and delete it
		deletedLike = l
		db = db.Where("id = ?", l.ID).Delete(&Like{})
		if db.Error != nil {
			return &Like{}, db.Error
		}
	}
	return deletedLike, nil
}

func (l *Like) GetLikesInfo(db *gorm.DB, mid uuid.UUID) (*[]Like, error) {

	likes := []Like{}
	err := db.Where("matchup_id = ?", mid).Find(&likes).Error
	if err != nil {
		return &[]Like{}, err
	}
	return &likes, err
}

// When a matchup is deleted, we also delete the likes that the matchup had
func (l *Like) DeleteUserLikes(db *gorm.DB, uid uuid.UUID) (int64, error) {
	db = db.Where("user_id = ?", uid).Delete(&Like{})
	if db.Error != nil {
		return 0, db.Error
	}
	return db.RowsAffected, nil
}

// When a matchup is deleted, we also delete the likes that the matchup had
func (l *Like) DeleteMatchupLikes(db *gorm.DB, mid uuid.UUID) (int64, error) {
	db = db.Where("matchup_id = ?", mid).Delete(&Like{})
	if db.Error != nil {
		return 0, db.Error
	}
	return db.RowsAffected, nil
}
