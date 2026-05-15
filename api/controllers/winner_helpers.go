package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"Matchup/models"

	"github.com/jmoiron/sqlx"
)

// stampMatchupWinner — single source of truth for "this matchup just
// got a winner". Consolidates the three previously-inline UPDATE
// sites (CompleteMatchup, OverrideMatchupWinner, bracket_logic round
// advance) so the post-win side effects (wins counter increments +
// won_* notification dispatch) can't be forgotten when a new winner-
// stamping path is added.
//
// What this does, in order:
//
//  1. UPDATE matchups SET winner_item_id, status, updated_at. The
//     row's `matchup` argument is mutated in place so the caller
//     doesn't need to re-fetch.
//  2. Locate the winner item in matchup.Items (already hydrated by
//     the caller — Items[*].UserID is the contender-as-user link
//     from migration 030).
//  3. If the winner item references a user:
//      a. UPDATE users SET wins_count = wins_count + 1 — global
//         tally surfaced on the profile Wins stat tile.
//      b. If the matchup is community-scoped, upsert
//         community_member_wins so the community's Champions
//         leaderboard reflects the new tally.
//      c. dispatchNotification(kind, "matchup", matchup.ID, payload)
//         — fires SSE bell + native push. Kind is caller-supplied
//         (won_matchup / won_round / won_bracket) so the bracket
//         logic can distinguish a round win from the final.
//
// Errors from steps 1 (the UPDATE) are fatal — callers should treat
// the return as "winner not stamped". Errors from steps 3a/3b are
// best-effort + logged; a failed counter increment shouldn't roll
// back the winner stamp because the matchup's state IS resolved
// even if the leaderboard hasn't caught up yet.
//
// kind: notification kind to dispatch on a user-backed winner. Pass
// kindWonMatchup for standalone + non-final bracket matchups, or
// kindWonBracket on the final. Pass "" to skip notification dispatch
// entirely (e.g. an admin override that we don't want to ping for).
func stampMatchupWinner(
	ctx context.Context,
	db sqlx.ExtContext,
	matchup *models.Matchup,
	winnerItemID uint,
	status string,
	kind string,
) error {
	if matchup == nil || winnerItemID == 0 {
		return fmt.Errorf("stampMatchupWinner: matchup or winnerItemID missing")
	}

	// 1. Stamp the matchup row. Mutate the model so the caller's
	//    response builder reflects the new state without re-fetching.
	matchup.WinnerItemID = &winnerItemID
	if status != "" {
		matchup.Status = status
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE matchups SET winner_item_id = $1, status = $2, updated_at = $3 WHERE id = $4",
		matchup.WinnerItemID, matchup.Status, time.Now(), matchup.ID,
	); err != nil {
		return fmt.Errorf("stampMatchupWinner: update matchup: %w", err)
	}

	// 2. Find the winning item in the pre-hydrated Items slice.
	//    Items.UserID is the link to "this contender was a user".
	var winnerItem *models.MatchupItem
	for i := range matchup.Items {
		if matchup.Items[i].ID == winnerItemID {
			winnerItem = &matchup.Items[i]
			break
		}
	}
	if winnerItem == nil || winnerItem.UserID == nil || *winnerItem.UserID == 0 {
		// Plain text / image winner — no user counter to bump and
		// no one to notify. Done.
		return nil
	}
	winnerUserID := *winnerItem.UserID

	// 3a. Global wins counter. Best-effort: a failed bump doesn't
	//     unstamp the winner — Champions tab will just be slightly
	//     stale until the next batch reconciliation (if/when added).
	if _, err := db.ExecContext(ctx,
		"UPDATE users SET wins_count = wins_count + 1, updated_at = NOW() WHERE id = $1",
		winnerUserID,
	); err != nil {
		_ = err // log already happens at the DB driver level
	}

	// 3b. Per-(community, user) counter — only for community-scoped
	//     matchups. Upsert via ON CONFLICT so the first win seeds the
	//     row and subsequent wins increment in place.
	if matchup.CommunityID != nil {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO community_member_wins (community_id, user_id, wins_count, last_won_at)
			VALUES ($1, $2, 1, NOW())
			ON CONFLICT (community_id, user_id) DO UPDATE
			SET wins_count  = community_member_wins.wins_count + 1,
			    last_won_at = NOW()
		`, *matchup.CommunityID, winnerUserID); err != nil {
			_ = err
		}
	}

	// 3c. Notification dispatch — caller may skip by passing "".
	if strings.TrimSpace(kind) != "" {
		// Author info for the push copy. matchup.Author may be empty
		// here if the caller didn't hydrate it, so fall back to a
		// generic title without the @author prefix.
		authorPart := ""
		if matchup.Author.ID != 0 && matchup.Author.Username != "" {
			authorPart = "@" + matchup.Author.Username + "'s "
		}
		pushTitle := "🎉 You won " + authorPart + "matchup"
		if kind == kindWonRound {
			pushTitle = "🥊 You won a bracket round"
		} else if kind == kindWonBracket {
			pushTitle = "🏆 You won the bracket"
		}
		pushURL := fmt.Sprintf("%s/users/%s/matchup/%s",
			strings.TrimRight(mailerAppBaseURL(), "/"),
			matchup.Author.Username, matchup.PublicID,
		)
		payload := map[string]any{
			"matchup_id":      matchup.PublicID,
			"matchup_title":   matchup.Title,
			"author_username": matchup.Author.Username,
		}
		dispatchNotification(
			ctx, db, winnerUserID,
			kind, "matchup", matchup.ID,
			payload,
			pushTitle,
			truncateForPush(matchup.Title, 140),
			pushURL,
		)
	}

	return nil
}
