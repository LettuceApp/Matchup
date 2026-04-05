package models

import "time"

type MatchupVote struct {
	ID                  uint      `db:"id" json:"id"`
	PublicID            string    `db:"public_id" json:"public_id"`
	UserID              *uint     `db:"user_id" json:"user_id,omitempty"`
	AnonID              *string   `db:"anon_id" json:"anon_id,omitempty"`
	MatchupPublicID     string    `db:"matchup_public_id" json:"matchup_public_id"`
	MatchupItemPublicID string    `db:"matchup_item_public_id" json:"matchup_item_public_id"`
	CreatedAt           time.Time `db:"created_at" json:"created_at"`
	UpdatedAt           time.Time `db:"updated_at" json:"updated_at"`
}
