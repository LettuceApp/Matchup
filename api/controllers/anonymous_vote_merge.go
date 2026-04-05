package controllers

import (
	"context"
	"database/sql"
	"errors"

	"Matchup/models"

	"github.com/jmoiron/sqlx"
)

func mergeDeviceVotesToUser(db *sqlx.DB, userID uint, deviceID string) error {
	if deviceID == "" {
		return nil
	}
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var votes []models.MatchupVote
	if err := sqlx.SelectContext(context.Background(), tx, &votes, "SELECT * FROM matchup_votes WHERE anon_id = $1", deviceID); err != nil {
		return err
	}

	for _, vote := range votes {
		var existing models.MatchupVote
		err := sqlx.GetContext(context.Background(), tx, &existing,
			"SELECT * FROM matchup_votes WHERE user_id = $1 AND matchup_public_id = $2", userID, vote.MatchupPublicID)
		if err == nil {
			// User already voted on this matchup — undo the anon vote
			var previousItem models.MatchupItem
			if err := sqlx.GetContext(context.Background(), tx, &previousItem,
				"SELECT * FROM matchup_items WHERE public_id = $1", vote.MatchupItemPublicID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(context.Background(),
				"UPDATE matchup_items SET votes = CASE WHEN votes > 0 THEN votes - 1 ELSE 0 END WHERE id = $1",
				previousItem.ID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(context.Background(),
				"DELETE FROM matchup_votes WHERE id = $1", vote.ID); err != nil {
				return err
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		// Reassign vote to user
		if _, err := tx.ExecContext(context.Background(),
			"UPDATE matchup_votes SET user_id = $1, anon_id = NULL WHERE id = $2",
			userID, vote.ID); err != nil {
			return err
		}
	}

	return tx.Commit()
}
