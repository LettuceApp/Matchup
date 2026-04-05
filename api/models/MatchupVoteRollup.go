package models

import "time"

type MatchupVoteRollup struct {
	MatchupID   uint      `db:"matchup_id" json:"matchup_id"`
	LegacyVotes int       `db:"legacy_votes" json:"legacy_votes"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}
