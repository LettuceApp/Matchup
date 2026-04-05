package controllers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"Matchup/models"

	"github.com/jmoiron/sqlx"
)

// ---- duration helpers ----

const (
	defaultMatchupDuration        = 24 * time.Hour
	minMatchupDurationSeconds int = 60
	maxMatchupDurationSeconds int = 86400
)

func normalizeMatchupDurationSeconds(seconds int) int {
	if seconds <= 0 {
		return int(defaultMatchupDuration.Seconds())
	}
	if seconds < minMatchupDurationSeconds {
		return minMatchupDurationSeconds
	}
	if seconds > maxMatchupDurationSeconds {
		return maxMatchupDurationSeconds
	}
	return seconds
}

// ---- winner determination (pure logic, no DB) ----

func determineMatchupWinner(matchup *models.Matchup) (uint, error) {
	if len(matchup.Items) == 0 {
		return 0, errors.New("matchup has no contenders")
	}
	if matchup.WinnerItemID != nil {
		for _, it := range matchup.Items {
			if it.ID == *matchup.WinnerItemID {
				return it.ID, nil
			}
		}
		return 0, errors.New("selected winner does not belong to this matchup")
	}

	maxVotes := matchup.Items[0].Votes
	winnerID := matchup.Items[0].ID
	topCount := 1
	for i := 1; i < len(matchup.Items); i++ {
		item := matchup.Items[i]
		if item.Votes > maxVotes {
			maxVotes = item.Votes
			winnerID = item.ID
			topCount = 1
		} else if item.Votes == maxVotes {
			topCount++
		}
	}
	if topCount > 1 {
		return 0, errors.New("matchup is tied. Select a winner before readying up")
	}
	return winnerID, nil
}

func determineMatchupWinnerByVotes(matchup *models.Matchup) (uint, error) {
	if len(matchup.Items) == 0 {
		return 0, errors.New("matchup has no contenders")
	}
	winnerID := matchup.Items[0].ID
	maxVotes := matchup.Items[0].Votes
	for _, item := range matchup.Items[1:] {
		if item.Votes > maxVotes {
			maxVotes = item.Votes
			winnerID = item.ID
		}
	}
	return winnerID, nil
}

func parseSeedValue(label string) (int, bool) {
	seed := 0
	found := false
	for _, r := range label {
		if r >= '0' && r <= '9' {
			seed = seed*10 + int(r-'0')
			found = true
		} else if found {
			break
		}
	}
	return seed, found
}

func determineMatchupWinnerByVotesOrSeed(matchup *models.Matchup) (uint, error) {
	if len(matchup.Items) == 0 {
		return 0, errors.New("matchup has no contenders")
	}

	maxVotes := matchup.Items[0].Votes
	tied := []models.MatchupItem{matchup.Items[0]}
	for _, item := range matchup.Items[1:] {
		if item.Votes > maxVotes {
			maxVotes = item.Votes
			tied = []models.MatchupItem{item}
		} else if item.Votes == maxVotes {
			tied = append(tied, item)
		}
	}

	if len(tied) == 1 {
		return tied[0].ID, nil
	}

	bestSeed := 0
	bestID := uint(0)
	for _, item := range tied {
		seed, ok := parseSeedValue(item.Item)
		if !ok {
			continue
		}
		if bestID == 0 || seed < bestSeed {
			bestSeed = seed
			bestID = item.ID
		}
	}
	if bestID != 0 {
		return bestID, nil
	}
	return 0, errors.New("matchup is tied")
}

// dedupeBracketMatchupItems merges duplicate items in bracket matchups. It tolerates
// up to 2 unique item keys and merges vote counts for duplicates.
func dedupeBracketMatchupItems(db sqlx.ExtContext, matchup *models.Matchup) error {
	if matchup.BracketID == nil || len(matchup.Items) <= 2 {
		return nil
	}

	type bucket struct {
		keep      *models.MatchupItem
		votes     int
		deleteIDs []uint
	}

	buckets := make(map[string]*bucket)
	for i := range matchup.Items {
		item := &matchup.Items[i]
		key := strings.TrimSpace(item.Item)
		if key == "" {
			key = fmt.Sprintf("id:%d", item.ID)
		}

		entry, ok := buckets[key]
		if !ok {
			buckets[key] = &bucket{keep: item, votes: item.Votes}
			continue
		}

		entry.votes += item.Votes
		if matchup.WinnerItemID != nil && item.ID == *matchup.WinnerItemID {
			entry.deleteIDs = append(entry.deleteIDs, entry.keep.ID)
			entry.keep = item
		} else {
			entry.deleteIDs = append(entry.deleteIDs, item.ID)
		}
	}

	if len(buckets) > 2 {
		return fmt.Errorf("matchup %d has too many items", matchup.ID)
	}

	ctx := context.Background()
	for _, entry := range buckets {
		if _, err := db.ExecContext(ctx, "UPDATE matchup_items SET votes = $1 WHERE id = $2", entry.votes, entry.keep.ID); err != nil {
			return err
		}
	}

	var deleteIDs []uint
	for _, entry := range buckets {
		deleteIDs = append(deleteIDs, entry.deleteIDs...)
	}
	if len(deleteIDs) > 0 {
		q, args, _ := sqlx.In("DELETE FROM matchup_items WHERE id IN (?)", deleteIDs)
		if rdb, ok := db.(interface{ Rebind(string) string }); ok {
			q = rdb.Rebind(q)
		}
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			return err
		}
	}

	return sqlx.SelectContext(ctx, db, &matchup.Items,
		"SELECT * FROM matchup_items WHERE matchup_id = $1 ORDER BY id ASC", matchup.ID)
}

// deleteMatchupCascade is the Server method wrapper.
// The actual logic is in deleteMatchupCascadeStandalone (matchup_connect_handler.go).
func (s *Server) deleteMatchupCascade(matchup *models.Matchup) error {
	return deleteMatchupCascadeStandalone(s.DB, matchup)
}
