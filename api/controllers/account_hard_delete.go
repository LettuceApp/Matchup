package controllers

import (
	"context"
	"fmt"

	"Matchup/models"

	"github.com/jmoiron/sqlx"
)

// Hard-delete cascade — the shared implementation behind three
// callers:
//
//   1. Admin DeleteUser — moderator-initiated immediate removal.
//   2. DeleteMyAccount + daily cron — runs once a user's 30-day
//      retention window elapses.
//   3. (Future) Admin ban escalation.
//
// Cascade order matters:
//   * Brackets first (owns matchups), then standalone matchups
//     (matchups owned by the user but NOT part of a bracket).
//   * Engagement tables (votes/likes/comments authored BY the user,
//     regardless of whether the content is owned by them) — the
//     user_id FK on these doesn't CASCADE; intentional design so
//     individual content removal doesn't wipe other users' context.
//   * Follow edges.
//   * User row — the CASCADE FKs on push_subscriptions +
//     refresh_tokens handle those tables automatically.
//
// Returns the first error encountered; subsequent DELETEs are NOT
// attempted so a partially-deleted user's remaining footprint is
// obvious in the logs.

func hardDeleteUser(ctx context.Context, db *sqlx.DB, userID uint) error {
	// Pull the user row first so we can call DeleteAUser at the end.
	var user models.User
	if err := sqlx.GetContext(ctx, db, &user,
		"SELECT * FROM users WHERE id = $1", userID,
	); err != nil {
		return fmt.Errorf("hardDeleteUser: load user %d: %w", userID, err)
	}

	// Brackets + their child matchups.
	var brackets []models.Bracket
	if err := sqlx.SelectContext(ctx, db, &brackets,
		"SELECT * FROM brackets WHERE author_id = $1", userID,
	); err != nil {
		return fmt.Errorf("hardDeleteUser: list brackets: %w", err)
	}
	s := &Server{DB: db}
	for i := range brackets {
		if err := s.deleteBracketCascade(&brackets[i]); err != nil {
			return fmt.Errorf("hardDeleteUser: cascade bracket %d: %w", brackets[i].ID, err)
		}
	}

	// Standalone matchups (bracket_id IS NULL — bracket children are
	// swept up by deleteBracketCascade already).
	var matchups []models.Matchup
	if err := sqlx.SelectContext(ctx, db, &matchups,
		"SELECT * FROM matchups WHERE author_id = $1 AND bracket_id IS NULL", userID,
	); err != nil {
		return fmt.Errorf("hardDeleteUser: list matchups: %w", err)
	}
	for i := range matchups {
		if err := s.deleteMatchupCascade(&matchups[i]); err != nil {
			return fmt.Errorf("hardDeleteUser: cascade matchup %d: %w", matchups[i].ID, err)
		}
	}

	// Engagement authored BY the user. These FKs don't CASCADE by
	// design — hard-deleting *content* shouldn't wipe every vote
	// on the content. Hard-deleting the *user* does.
	for _, query := range []string{
		"DELETE FROM matchup_votes   WHERE user_id = $1",
		"DELETE FROM likes           WHERE user_id = $1",
		"DELETE FROM bracket_likes   WHERE user_id = $1",
		"DELETE FROM bracket_comments WHERE user_id = $1",
		"DELETE FROM comments        WHERE user_id = $1",
	} {
		if _, err := db.ExecContext(ctx, query, userID); err != nil {
			return fmt.Errorf("hardDeleteUser: %s: %w", query, err)
		}
	}

	// Follow edges — no CASCADE on `follows` FKs (follows table
	// predates the moderation work).
	if err := removeUserFollowEdges(db, userID); err != nil {
		return fmt.Errorf("hardDeleteUser: remove follow edges: %w", err)
	}

	// User row drop. push_subscriptions + refresh_tokens CASCADE
	// via their FKs; no additional cleanup needed.
	if _, err := user.DeleteAUser(db, userID); err != nil {
		return fmt.Errorf("hardDeleteUser: drop user row: %w", err)
	}
	return nil
}

// HardDeleteExpiredAccounts is the cron target. Scans for users
// whose deleted_at is past the retention window, then hard-deletes
// each via the shared cascade. Batched to LIMIT 100 so a one-time
// spike of self-deletions doesn't blow past the job's timeout; the
// next tick picks up the remainder.
//
// Returns the number of users actually hard-deleted.
func HardDeleteExpiredAccounts(ctx context.Context, db *sqlx.DB) (int, error) {
	var candidateIDs []uint
	if err := sqlx.SelectContext(ctx, db, &candidateIDs, `
		SELECT id
		  FROM public.users
		 WHERE deleted_at IS NOT NULL
		   AND deleted_at < NOW() - ($1::interval)
		 LIMIT 100`,
		fmt.Sprintf("%d seconds", int(AccountHardDeleteRetention.Seconds())),
	); err != nil {
		return 0, fmt.Errorf("HardDeleteExpiredAccounts: scan: %w", err)
	}

	count := 0
	for _, uid := range candidateIDs {
		if err := hardDeleteUser(ctx, db, uid); err != nil {
			// Log and keep going — one bad row shouldn't block the
			// rest of the batch. The scheduler wrapper prints the
			// per-user error.
			return count, err
		}
		count++
	}
	return count, nil
}
