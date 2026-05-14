package controllers

import (
	"context"
	"fmt"
	"log"
	"regexp"
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
// generateFullBracket creates the bracket's matchup skeleton — round 1
// matchups carry items from the caller-supplied entries; later rounds
// stay empty until prior rounds resolve.
//
// User-as-item: when an entry string matches `@<username>` (or `@me`,
// which resolves to the creator), the resulting matchup_item gets
// user_id set and the item label falls back to the resolved
// @username. Entries that don't match the @-handle pattern stay as
// plain text labels via formatSeedLabel. The author + the resolved
// user info are surfaced on the bracket page the same way standalone
// matchups handle them.
//
// Notifications: each unique resolved user (other than the creator)
// gets a mentioned_as_item dispatch after the round-1 matchups land
// — same kind + payload as the standalone matchup path so a viewer's
// bell treats them uniformly.
func generateFullBracket(ctx context.Context, db *sqlx.DB, bracket models.Bracket, entries []string) {
	totalRounds := 0
	for (1 << totalRounds) < bracket.Size {
		totalRounds++
	}

	// Resolve all user-handle entries up front so we can build the
	// round-1 items with their UserID + display fallback in one pass.
	// Failures (handle didn't resolve, e.g. typo) degrade gracefully:
	// the entry stays as a plain text label, no notification fires.
	type resolvedEntry struct {
		text   string  // display label (@username for resolved, raw text otherwise)
		userID *uint   // non-nil when this entry is a user
		user   *models.User
	}
	resolved := make([]resolvedEntry, len(entries))
	for idx, raw := range entries {
		clean := strings.TrimSpace(raw)
		if m := mentionEntryRegex.FindStringSubmatch(clean); m != nil {
			if u, err := resolveUserHandle(ctx, db, m[0], bracket.AuthorID); err == nil && u != nil {
				uid := u.ID
				resolved[idx] = resolvedEntry{
					text:   "@" + u.Username,
					userID: &uid,
					user:   u,
				}
				continue
			}
		}
		resolved[idx] = resolvedEntry{text: clean}
	}

	// Collect unique notification targets so the same user listed
	// twice in entries only fires one push. Creator is excluded
	// (they know they added themselves via @me).
	notifyTargets := map[uint]string{} // user_id → username
	for _, r := range resolved {
		if r.userID == nil || *r.userID == 0 || *r.userID == bracket.AuthorID {
			continue
		}
		notifyTargets[*r.userID] = r.user.Username
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
				var entryA, entryB resolvedEntry
				if seedA-1 < len(resolved) {
					entryA = resolved[seedA-1]
				}
				if seedB-1 < len(resolved) {
					entryB = resolved[seedB-1]
				}
				m.Items = []models.MatchupItem{
					{Item: formatSeedLabel(seedA, entryA.text), UserID: entryA.userID},
					{Item: formatSeedLabel(seedB, entryB.text), UserID: entryB.userID},
				}
			}
			m.Prepare()
			if _, err := m.SaveMatchup(db); err != nil {
				log.Printf("generateFullBracket: failed to save matchup round %d match %d: %v", round, i+1, err)
			}
		}
	}

	// Fire mentioned_as_item notifications once per unique user-
	// contender. Best-effort — a failure doesn't roll back the
	// bracket creation. Push copy mirrors the standalone matchup
	// path so the recipient's tray reads consistently.
	if len(notifyTargets) > 0 {
		var author models.User
		_ = sqlx.GetContext(ctx, db, &author, "SELECT * FROM users WHERE id = $1", bracket.AuthorID)
		pushURL := fmt.Sprintf("%s/brackets/%s",
			strings.TrimRight(mailerAppBaseURL(), "/"),
			bracket.PublicID,
		)
		payload := map[string]any{
			"bracket_id":      bracket.PublicID,
			"bracket_title":   bracket.Title,
			"author_username": author.Username,
		}
		for uid := range notifyTargets {
			dispatchNotification(
				ctx, db, uid,
				kindMentionedAsItem, "bracket", bracket.ID,
				payload,
				"@"+author.Username+" added you to a bracket",
				truncateForPush(bracket.Title, 140),
				pushURL,
			)
		}
	}
}

// mentionEntryRegex matches a bare `@<username>` entry — what a
// creator types in a bracket contender row to reference a user.
// Mirrors the frontend's `/^@([A-Za-z0-9_]+)$/` parser in
// CreateMatchup so the wire contract stays consistent.
var mentionEntryRegex = regexp.MustCompile(`^@[A-Za-z0-9_]+$`)

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

	// Auto-finalize unfinished current-round matchups. Two triggers:
	//   1. Timer mode + the round end-time has passed (existing behavior).
	//   2. Manual mode at all (NEW). Owners reported that clicking Advance
	//      with decisive 1-0 matchups did nothing — the silent-fail was
	//      because manual mode previously required the owner to Ready up
	//      each matchup individually, then click Advance. Click Advance
	//      now auto-picks winners by votes so the owner's mental model
	//      ("decide the round and move on") matches behavior. Genuine ties
	//      still fail via determineMatchupWinnerByVotesOrSeed's tie error,
	//      which the frontend surfaces as a toast pointing at Override.
	roundExpired := bracket.AdvanceMode == "timer" &&
		bracket.RoundEndsAt != nil &&
		!time.Now().Before(*bracket.RoundEndsAt)
	isManualMode := bracket.AdvanceMode == "manual"
	shouldAutoFinalize := roundExpired || isManualMode

	if shouldAutoFinalize {
		now := time.Now()
		for i := range currentMatchups {
			m := &currentMatchups[i]
			if m.Status == matchupStatusCompleted && m.WinnerItemID != nil {
				continue
			}
			winnerID, err := determineMatchupWinnerByVotesOrSeed(m)
			if err != nil {
				// In manual mode rewrap the tie error with copy that
				// points at Override winner — the only path out of a
				// genuine tie. Timer mode keeps its original error
				// shape so the scheduler's logging stays consistent.
				if isManualMode {
					return false, fmt.Errorf("matchup %d tied — pick a winner via Override before advancing", m.ID)
				}
				return false, err
			}
			// Round-advance: fires kindWonRound on user-backed winners.
			// Items + author are pre-hydrated by the caller so the
			// helper can find the winning item + push copy.
			if err := stampMatchupWinner(ctx, tx, m, winnerID, matchupStatusCompleted, kindWonRound); err != nil {
				return false, err
			}
			_ = now // helper stamps its own NOW(); kept symbolic.
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
			// Final round: fires kindWonBracket — distinct kind so the
			// notification copy can read "🏆 You won the bracket"
			// instead of the per-round "you won a round" flavor.
			if err := stampMatchupWinner(ctx, tx, m, winnerID, matchupStatusCompleted, kindWonBracket); err != nil {
				return false, err
			}
			_ = now
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
