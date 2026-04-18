package controllers

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

// ---- snapshot types (scanned from materialized views) ----

type HomeSummarySnapshot struct {
	VotesToday     int64 `db:"votes_today"`
	ActiveMatchups int64 `db:"active_matchups"`
	ActiveBrackets int64 `db:"active_brackets"`
}

type HomeMatchupSnapshot struct {
	ID         uint      `db:"id"`
	Title      string    `db:"title"`
	AuthorID   uint      `db:"author_id"`
	BracketID  *uint     `db:"bracket_id"`
	Visibility string    `db:"visibility"`
	CreatedAt  time.Time `db:"created_at"`
	Rank       int32     `db:"rank"`
}

type HomeCreatorSnapshot struct {
	ID             uint   `db:"id"`
	Username       string `db:"username"`
	AvatarPath     string `db:"avatar_path"`
	FollowersCount int64  `db:"followers_count"`
	FollowingCount int64  `db:"following_count"`
	IsPrivate      bool   `db:"is_private"`
	Rank           int32  `db:"rank"`
}

// calculateUserEngagement returns a weighted engagement score for the given author.
func (s *Server) calculateUserEngagement(userID uint) (float64, error) {
	ctx := context.Background()

	var totalVotes int64
	if err := sqlx.GetContext(ctx, s.DB, &totalVotes, `
		SELECT COALESCE(SUM(mi.votes), 0)
		FROM matchup_items mi
		JOIN matchups m ON m.id = mi.matchup_id
		WHERE m.author_id = $1`, userID); err != nil {
		return 0, err
	}

	var selfVotes int64
	if err := sqlx.GetContext(ctx, s.DB, &selfVotes, `
		SELECT COALESCE(COUNT(*), 0)
		FROM matchup_votes mv
		JOIN matchups m ON m.id = mv.matchup_id
		WHERE m.author_id = $1 AND mv.user_id = $2`, userID, userID); err != nil {
		return 0, err
	}

	var likes int64
	if err := sqlx.GetContext(ctx, s.DB, &likes, `
		SELECT COALESCE(COUNT(*), 0)
		FROM likes l
		JOIN matchups m ON m.id = l.matchup_id
		WHERE m.author_id = $1 AND l.user_id <> $2`, userID, userID); err != nil {
		return 0, err
	}

	var comments int64
	if err := sqlx.GetContext(ctx, s.DB, &comments, `
		SELECT COALESCE(COUNT(*), 0)
		FROM comments c
		JOIN matchups m ON m.id = c.matchup_id
		WHERE m.author_id = $1 AND c.user_id <> $2`, userID, userID); err != nil {
		return 0, err
	}

	var bracketLikes int64
	if err := sqlx.GetContext(ctx, s.DB, &bracketLikes, `
		SELECT COALESCE(COUNT(*), 0)
		FROM bracket_likes bl
		JOIN brackets b ON b.id = bl.bracket_id
		WHERE b.author_id = $1 AND bl.user_id <> $2`, userID, userID); err != nil {
		return 0, err
	}

	var bracketComments int64
	if err := sqlx.GetContext(ctx, s.DB, &bracketComments, `
		SELECT COALESCE(COUNT(*), 0)
		FROM bracket_comments bc
		JOIN brackets b ON b.id = bc.bracket_id
		WHERE b.author_id = $1 AND bc.user_id <> $2`, userID, userID); err != nil {
		return 0, err
	}

	votes := totalVotes - selfVotes
	if votes < 0 {
		votes = 0
	}

	return float64(votes)*2 +
		float64(likes)*3 +
		float64(comments)*0.5 +
		float64(bracketLikes)*3 +
		float64(bracketComments)*0.5, nil
}
