package models

import (
	"strings"
	"time"

	"github.com/twinj/uuid"
	"gorm.io/gorm"
)

type MatchupVote struct {
	ID                  uint      `gorm:"primary_key;autoIncrement" json:"id"`
	PublicID            string    `gorm:"type:uuid;uniqueIndex;column:public_id" json:"public_id"`
	UserID              *uint     `gorm:"index;uniqueIndex:idx_matchup_vote_user_matchup_public" json:"user_id,omitempty"`
	AnonID              *string   `gorm:"size:36;index;uniqueIndex:idx_matchup_vote_anon_matchup" json:"anon_id,omitempty"`
	MatchupPublicID     string    `gorm:"type:uuid;not null;index;uniqueIndex:idx_matchup_vote_user_matchup_public;uniqueIndex:idx_matchup_vote_anon_matchup" json:"matchup_public_id"`
	MatchupItemPublicID string    `gorm:"type:uuid;not null;index" json:"matchup_item_public_id"`
	CreatedAt           time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (vote *MatchupVote) BeforeCreate(tx *gorm.DB) (err error) {
	if strings.TrimSpace(vote.PublicID) == "" {
		vote.PublicID = uuid.NewV4().String()
	}
	return nil
}
