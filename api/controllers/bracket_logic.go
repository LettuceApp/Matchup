package controllers

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"Matchup/cache"
	appdb "Matchup/db"
	"Matchup/models"

	"github.com/jmoiron/sqlx"
)

// ---- small helpers ----

func ptrUint(v uint) *uint { return &v }
func ptrInt(v int) *int    { return &v }

const defaultRoundDurationSeconds = 86400

// ---- bracket round window ----

func setBracketRoundWindow(bracket *models.Bracket, now time.Time) {
	if bracket.AdvanceMode != "timer" {
		bracket.RoundStartedAt = nil
		bracket.RoundEndsAt = nil
		return
	}
	if bracket.RoundDurationSeconds <= 0 {
		bracket.RoundDurationSeconds = defaultRoundDurationSeconds
	}
	bracket.RoundStartedAt = &now
	end := now.Add(time.Duration(bracket.RoundDurationSeconds) * time.Second)
	bracket.RoundEndsAt = &end
}

// ---- bracket round sync ----

// syncBracketMatchups updates matchup statuses to reflect the bracket's current round.
// Call within a transaction after advancing bracket.CurrentRound.
func syncBracketMatchups(db sqlx.ExtContext, bracket *models.Bracket) error {
	ctx := context.Background()

	// Past rounds → completed
	if _, err := db.ExecContext(ctx,
		"UPDATE matchups SET status = $1, updated_at = $2 WHERE bracket_id = $3 AND round < $4",
		matchupStatusCompleted, time.Now(), bracket.ID, bracket.CurrentRound,
	); err != nil {
		return err
	}

	// Current round → active
	startTime := (*time.Time)(nil)
	endTime := (*time.Time)(nil)
	if bracket.AdvanceMode == "timer" {
		startTime = bracket.RoundStartedAt
		endTime = bracket.RoundEndsAt
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE matchups SET status = $1, start_time = $2, end_time = $3, updated_at = $4 WHERE bracket_id = $5 AND round = $6",
		matchupStatusActive, startTime, endTime, time.Now(), bracket.ID, bracket.CurrentRound,
	); err != nil {
		return err
	}

	// Future rounds → draft, clear times
	if _, err := db.ExecContext(ctx,
		"UPDATE matchups SET status = $1, start_time = NULL, end_time = NULL, updated_at = $2 WHERE bracket_id = $3 AND round > $4",
		matchupStatusDraft, time.Now(), bracket.ID, bracket.CurrentRound,
	); err != nil {
		return err
	}

	return nil
}

// ---- bracket generation ----

// groupMatchupsByRound organises a flat matchup list into a map keyed by round number.
func groupMatchupsByRound(matchups []models.Matchup) map[int][]models.Matchup {
	rounds := make(map[int][]models.Matchup)
	for _, m := range matchups {
		if m.Round != nil {
			rounds[*m.Round] = append(rounds[*m.Round], m)
		}
	}
	return rounds
}

// formatSeedLabel builds a "Seed N - Name" label, stripping any pre-existing seed prefix.
func formatSeedLabel(seed int, name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Sprintf("Seed %d", seed)
	}
	trimmed = stripSeedPrefix(trimmed)
	if trimmed == "" {
		return fmt.Sprintf("Seed %d", seed)
	}
	return fmt.Sprintf("Seed %d - %s", seed, trimmed)
}

func stripSeedPrefix(label string) string {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" {
		return ""
	}
	for {
		lower := strings.ToLower(trimmed)
		if !strings.HasPrefix(lower, "seed") {
			return trimmed
		}
		i := len("seed")
		for i < len(trimmed) && trimmed[i] == ' ' {
			i++
		}
		startDigits := i
		for i < len(trimmed) && trimmed[i] >= '0' && trimmed[i] <= '9' {
			i++
		}
		if startDigits == i {
			return trimmed
		}
		for i < len(trimmed) && trimmed[i] == ' ' {
			i++
		}
		if i < len(trimmed) && (trimmed[i] == '-' || trimmed[i] == ':') {
			i++
			for i < len(trimmed) && trimmed[i] == ' ' {
				i++
			}
		}
		trimmed = strings.TrimSpace(trimmed[i:])
		if trimmed == "" {
			return ""
		}
	}
}

// generateFullBracket creates all matchup slots for a bracket.
// Round 1 matchups get seed-labeled items; later rounds are empty placeholders.
func generateFullBracket(db *sqlx.DB, bracket models.Bracket, entries []string) {
	totalRounds := 0
	for (1 << totalRounds) < bracket.Size {
		totalRounds++
	}

	for round := 1; round <= totalRounds; round++ {
		matchupsThisRound := bracket.Size / (1 << round)
		for i := 0; i < matchupsThisRound; i++ {
			m := models.Matchup{
				Title:     fmt.Sprintf("Round %d - Match %d", round, i+1),
				Content:   "Bracket matchup",
				AuthorID:  bracket.AuthorID,
				BracketID: &bracket.ID,
				Round:     ptrInt(round),
				Seed:      ptrInt(i + 1),
				Status:    matchupStatusDraft,
			}
			if round == 1 {
				seedA := i + 1
				seedB := bracket.Size - i
				var nameA, nameB string
				if seedA-1 < len(entries) {
					nameA = entries[seedA-1]
				}
				if seedB-1 < len(entries) {
					nameB = entries[seedB-1]
				}
				m.Items = []models.MatchupItem{
					{Item: formatSeedLabel(seedA, nameA)},
					{Item: formatSeedLabel(seedB, nameB)},
				}
			}
			m.Prepare()
			if _, err := m.SaveMatchup(db); err != nil {
				log.Printf("generateFullBracket: failed to save matchup round %d match %d: %v", round, i+1, err)
			}
		}
	}
}

// ---- cascade deletes ----

// deleteBracketCascade removes a bracket and all its associated data.
func (s *Server) deleteBracketCascade(bracket *models.Bracket) error {
	matchups, err := models.FindMatchupsByBracket(s.DB, bracket.ID)
	if err != nil {
		return err
	}
	for i := range matchups {
		if err := s.deleteMatchupCascade(&matchups[i]); err != nil {
			return err
		}
	}

	ctx := context.Background()
	if _, err := s.DB.ExecContext(ctx, "DELETE FROM bracket_likes WHERE bracket_id = $1", bracket.ID); err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, "DELETE FROM bracket_comments WHERE bracket_id = $1", bracket.ID); err != nil {
		return err
	}
	if _, err := bracket.DeleteBracket(s.DB, bracket.ID); err != nil {
		return err
	}
	invalidateBracketSummaryCache(bracket.ID)
	invalidateHomeSummaryCache(bracket.AuthorID)
	return nil
}

// ---- bracket advance ----

// advanceBracketInternal advances the bracket to the next round.
// It runs inside its own transaction with a row-level lock on the bracket.
func (s *Server) advanceBracketInternal(db *sqlx.DB, bracket *models.Bracket) (bool, error) {
	tx, err := db.Beginx()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	ctx := context.Background()

	// Lock the bracket row for the duration of the transaction.
	var locked models.Bracket
	if err := tx.QueryRowxContext(ctx,
		"SELECT * FROM brackets WHERE id = $1 FOR UPDATE", bracket.ID,
	).StructScan(&locked); err != nil {
		return false, err
	}
	*bracket = locked

	currentRound := bracket.CurrentRound
	nextRound := currentRound + 1

	// Load all matchups with items for this bracket.
	matchups, err := models.FindMatchupsByBracket(tx, bracket.ID)
	if err != nil {
		return false, err
	}

	// Merge any pending vote deltas from the Redis buffer onto the
	// freshly-loaded matchup items. Without this, an in-progress flush
	// could let the bracket pick the wrong winner — the votes have been
	// cast (rows exist in matchup_votes) but the matchup_items.votes
	// counter is still catching up. Mutating the items in place here
	// flows through to the round buckets below because Go slice copies
	// share their backing array.
	applyVoteDeltasToMatchups(ctx, matchups)

	rounds := groupMatchupsByRound(matchups)
	currentMatchups := rounds[currentRound]
	if len(currentMatchups) == 0 {
		return false, fmt.Errorf("no matchups found for current round")
	}

	// Deduplicate items in each current-round matchup.
	for i := range currentMatchups {
		if err := dedupeBracketMatchupItems(tx, &currentMatchups[i]); err != nil {
			return false, err
		}
	}

	nextMatchups := rounds[nextRound]

	// If on a timer and the round has expired, auto-determine winners.
	roundExpired := bracket.AdvanceMode == "timer" &&
		bracket.RoundEndsAt != nil &&
		!time.Now().Before(*bracket.RoundEndsAt)

	if roundExpired {
		now := time.Now()
		for i := range currentMatchups {
			m := &currentMatchups[i]
			if m.Status == matchupStatusCompleted && m.WinnerItemID != nil {
				continue
			}
			winnerID, err := determineMatchupWinnerByVotesOrSeed(m)
			if err != nil {
				return false, err
			}
			if _, err := tx.ExecContext(ctx,
				"UPDATE matchups SET winner_item_id = $1, status = $2, updated_at = $3 WHERE id = $4",
				winnerID, matchupStatusCompleted, now, m.ID,
			); err != nil {
				return false, err
			}
			m.WinnerItemID = &winnerID
			m.Status = matchupStatusCompleted
		}
	}

	// Prevent advancing if next round already has items populated.
	for _, nm := range nextMatchups {
		if len(nm.Items) > 0 {
			return false, fmt.Errorf("next round already populated")
		}
	}

	// Final round → auto-complete any unfinished matchups, then mark bracket completed.
	if len(nextMatchups) == 0 {
		now := time.Now()
		for i := range currentMatchups {
			m := &currentMatchups[i]
			if m.Status == matchupStatusCompleted && m.WinnerItemID != nil {
				continue
			}
			winnerID, err := determineMatchupWinnerByVotesOrSeed(m)
			if err != nil {
				return false, err
			}
			if _, err := tx.ExecContext(ctx,
				"UPDATE matchups SET winner_item_id = $1, status = $2, updated_at = $3 WHERE id = $4",
				winnerID, matchupStatusCompleted, now, m.ID,
			); err != nil {
				return false, err
			}
		}
		bracket.Status = "completed"
		bracket.CompletedAt = &now
		if _, err := bracket.UpdateBracket(tx); err != nil {
			return false, err
		}
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return true, nil
	}

	// Collect winner item labels from current round.
	winners := make([]string, 0, len(currentMatchups))
	for _, m := range currentMatchups {
		if m.Status != matchupStatusCompleted {
			return false, fmt.Errorf("matchup %d not completed", m.ID)
		}
		winner, err := m.WinnerItem()
		if err != nil {
			return false, err
		}
		winners = append(winners, winner.Item)
	}

	if len(winners) != len(nextMatchups)*2 {
		return false, fmt.Errorf(
			"invalid bracket state: expected %d winners, got %d",
			len(nextMatchups)*2, len(winners),
		)
	}

	// Populate next round matchups with winner items.
	w := 0
	for i := range nextMatchups {
		nm := &nextMatchups[i]
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM matchup_items WHERE matchup_id = $1", nm.ID,
		); err != nil {
			return false, err
		}
		for _, itemLabel := range []string{winners[w], winners[w+1]} {
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO matchup_items (public_id, matchup_id, item, votes) VALUES ($1, $2, $3, 0)",
				appdb.GeneratePublicID(), nm.ID, itemLabel,
			); err != nil {
				return false, err
			}
		}
		w += 2
	}

	// Advance the round counter and update the bracket.
	bracket.CurrentRound = nextRound
	setBracketRoundWindow(bracket, time.Now())
	if _, err := bracket.UpdateBracket(tx); err != nil {
		return false, err
	}

	// Sync matchup statuses to reflect the new current round.
	if err := syncBracketMatchups(tx, bracket); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}

	if cache.Client != nil {
		cache.Client.Publish(context.Background(),
			fmt.Sprintf("bracket:%d:events", bracket.ID), "advance")
	}

	return true, nil
}
