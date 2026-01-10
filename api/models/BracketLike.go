package models

import (
	"time"

	"gorm.io/gorm"
)

type BracketLike struct {
	ID        uint      `gorm:"primary_key;autoIncrement" json:"id"`
	UserID    uint      `gorm:"not null" json:"user_id"`
	BracketID uint      `gorm:"not null" json:"bracket_id"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (l *BracketLike) DeleteUserBracketLikes(db *gorm.DB, uid uint) (int64, error) {
	db = db.Where("user_id = ?", uid).Delete(&BracketLike{})
	if db.Error != nil {
		return 0, db.Error
	}
	return db.RowsAffected, nil
}

func (l *BracketLike) DeleteBracketLikes(db *gorm.DB, bid uint) (int64, error) {
	db = db.Where("bracket_id = ?", bid).Delete(&BracketLike{})
	if db.Error != nil {
		return 0, db.Error
	}
	return db.RowsAffected, nil
}
