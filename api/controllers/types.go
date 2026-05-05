package controllers

import "time"

// ---- popular matchup row (scanned from popular_matchups_snapshot view) ----

type popularMatchupRow struct {
	ID              uint     `db:"id"`
	Title           string   `db:"title"`
	AuthorID        uint     `db:"author_id"`
	BracketID       *uint    `db:"bracket_id"`
	BracketAuthorID *uint    `db:"bracket_author_id"`
	Round           *int     `db:"round"`
	CurrentRound    *int     `db:"current_round"`
	Votes           int64    `db:"votes"`
	Likes           int64    `db:"likes"`
	Comments        int64    `db:"comments"`
	EngagementScore float64  `db:"engagement_score"`
	Rank            int64    `db:"rank"`
}

// ---- popular bracket row (scanned from popular_brackets_snapshot view) ----

type popularBracketRow struct {
	ID              uint    `db:"id"`
	Title           string  `db:"title"`
	AuthorID        uint    `db:"author_id"`
	CurrentRound    int     `db:"current_round"`
	EngagementScore float64 `db:"engagement_score"`
	Rank            int64   `db:"rank"`
}

// ---- popular DTOs (public-id-mapped intermediary types) ----

type PopularMatchupDTO struct {
	ID              string
	Title           string
	AuthorID        string
	AuthorUsername  string
	BracketID       *string
	BracketAuthorID *string
	Round           *int
	CurrentRound    *int
	Votes           int64
	Likes           int64
	Comments        int64
	EngagementScore float64
	Rank            int64
	CreatedAt       string
}

type PopularBracketDTO struct {
	ID              string
	Title           string
	AuthorID        string
	AuthorUsername  string
	CurrentRound    int
	Size            int
	Votes           int64
	Likes           int64
	Comments        int64
	EngagementScore float64
	Rank            int64
	CreatedAt       string
}

// ---- follow types ----

// followRow is scanned from the join of follows and users.
// The query enumerates each users column explicitly (rather than
// `SELECT users.*`) — sqlx's strict scan errors on extras, and the
// users table has grown to include columns we don't ferry through
// the follow list (password hash, notification_prefs, deleted_at,
// banned_at, email_verified_at, etc.). See
// fetchFollowRowsStandalone in user_connect_handler.go for the
// canonical column list — adding a field here means adding the
// matching column there.
type followRow struct {
	// follows-specific fields
	FollowID        uint      `db:"follow_id"`
	FollowCreatedAt time.Time `db:"follow_created_at"`
	// users fields
	UserID         uint      `db:"id"`
	PublicID       string    `db:"public_id"`
	Username       string    `db:"username"`
	Email          string    `db:"email"`
	AvatarPath     string    `db:"avatar_path"`
	IsAdmin        bool      `db:"is_admin"`
	IsPrivate      bool      `db:"is_private"`
	FollowersCount int64     `db:"followers_count"`
	FollowingCount int64     `db:"following_count"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

type followCursor struct {
	ID        uint
	CreatedAt time.Time
}

// ---- follow user DTO ----

type FollowUserDTO struct {
	ID               string
	Username         string
	Email            string
	AvatarPath       string
	IsAdmin          bool
	FollowersCount   int
	FollowingCount   int
	ViewerFollowing  bool
	ViewerFollowedBy bool
	Mutual           bool
}
