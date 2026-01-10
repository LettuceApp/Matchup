package models

import "time"

type MatchupVote struct {
	ID            uint      `gorm:"primary_key;autoIncrement" json:"id"`
	UserID        uint      `gorm:"not null;index;uniqueIndex:idx_matchup_vote_user_matchup" json:"user_id"`
	MatchupID     uint      `gorm:"not null;index;uniqueIndex:idx_matchup_vote_user_matchup" json:"matchup_id"`
	MatchupItemID uint      `gorm:"not null;index" json:"matchup_item_id"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
