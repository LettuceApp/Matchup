package models

import "time"

type MatchupVoteRollup struct {
	MatchupID   uint      `gorm:"primaryKey" json:"matchup_id"`
	LegacyVotes int       `gorm:"not null;default:0" json:"legacy_votes"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
