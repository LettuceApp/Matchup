package models

import (
	"time"

	"gorm.io/gorm"
)

type Like struct {
	ID        uint      `gorm:"primary_key;autoIncrement" json:"id"`
	UserID    uint      `gorm:"not null" json:"user_id"`
	MatchupID uint      `gorm:"not null" json:"matchup_id"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (like *Like) SaveLike(db *gorm.DB) (*Like, error) {
	err := db.Create(&like).Error
	if err != nil {
		return nil, err
	}
	return like, nil
}

func (l *Like) DeleteLike(db *gorm.DB) (*Like, error) {
	var err error
	var deletedLike *Like

	err = db.Where("id = ?", l.ID).First(&l).Error
	if err != nil {
		return &Like{}, err
	} else {
		// If the like exists, save it in deletedLike and delete it
		deletedLike = l
		db = db.Where("id = ?", l.ID).Delete(&Like{})
		if db.Error != nil {
			return &Like{}, db.Error
		}
	}
	return deletedLike, nil
}

func (l *Like) GetLikesInfo(db *gorm.DB, mid uint) (*[]Like, error) {
	likes := []Like{}
	err := db.Where("matchup_id = ?", mid).Find(&likes).Error
	if err != nil {
		return &[]Like{}, err
	}
	return &likes, err
}

// When a user is deleted, we also delete the likes that the user had
func (l *Like) DeleteUserLikes(db *gorm.DB, uid uint) (int64, error) {
	db = db.Where("user_id = ?", uid).Delete(&Like{})
	if db.Error != nil {
		return 0, db.Error
	}
	return db.RowsAffected, nil
}

// When a matchup is deleted, we also delete the likes that the matchup had
func (l *Like) DeleteMatchupLikes(db *gorm.DB, mid uint) (int64, error) {
	db = db.Where("matchup_id = ?", mid).Delete(&Like{})
	if db.Error != nil {
		return 0, db.Error
	}
	return db.RowsAffected, nil
}
