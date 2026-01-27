package controllers

import "time"

type UserDTO struct {
	ID             string    `json:"id"`
	Username       string    `json:"username"`
	Email          string    `json:"email"`
	AvatarPath     string    `json:"avatar_path"`
	IsAdmin        bool      `json:"is_admin"`
	IsPrivate      bool      `json:"is_private"`
	FollowersCount int       `json:"followers_count"`
	FollowingCount int       `json:"following_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type UserSummaryDTO struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type FollowUserDTO struct {
	UserDTO
	ViewerFollowing  bool `json:"viewer_following"`
	ViewerFollowedBy bool `json:"viewer_followed_by"`
	Mutual           bool `json:"mutual"`
}

type MatchupItemDTO struct {
	ID    string `json:"id"`
	Item  string `json:"item"`
	Votes int    `json:"votes"`
}

type CommentDTO struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type MatchupDTO struct {
	ID              string           `json:"id"`
	Title           string           `json:"title"`
	Content         string           `json:"content"`
	AuthorID        string           `json:"author_id"`
	Author          UserSummaryDTO   `json:"author"`
	Items           []MatchupItemDTO `json:"items"`
	Comments        []CommentDTO     `json:"comments"`
	LikesCount      int64            `json:"likes_count"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
	BracketID       *string          `json:"bracket_id"`
	Round           *int             `json:"round"`
	Seed            *int             `json:"seed"`
	Status          string           `json:"status"`
	EndMode         string           `json:"end_mode"`
	DurationSeconds int              `json:"duration_seconds"`
	StartTime       *time.Time       `json:"start_time"`
	EndTime         *time.Time       `json:"end_time"`
	WinnerItemID    *string          `json:"winner_item_id"`
}

type BracketDTO struct {
	ID                   string     `json:"id"`
	Title                string     `json:"title"`
	Description          string     `json:"description"`
	AuthorID             string     `json:"author_id"`
	Size                 int        `json:"size"`
	Status               string     `json:"status"`
	CurrentRound         int        `json:"current_round"`
	AdvanceMode          string     `json:"advance_mode"`
	RoundDurationSeconds int        `json:"round_duration_seconds"`
	RoundStartedAt       *time.Time `json:"round_started_at"`
	RoundEndsAt          *time.Time `json:"round_ends_at"`
	LikesCount           int64      `json:"likes_count"`
}

type LikeDTO struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	MatchupID string    `json:"matchup_id"`
	CreatedAt time.Time `json:"created_at"`
}

type BracketLikeDTO struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	BracketID string    `json:"bracket_id"`
	CreatedAt time.Time `json:"created_at"`
}

type MatchupVoteDTO struct {
	ID            string    `json:"id"`
	UserID        *string   `json:"user_id"`
	MatchupID     string    `json:"matchup_id"`
	MatchupItemID string    `json:"matchup_item_id"`
	CreatedAt     time.Time `json:"created_at"`
}

type PopularMatchupDTO struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	AuthorID        string   `json:"author_id"`
	BracketID       *string  `json:"bracket_id"`
	BracketAuthorID *string  `json:"bracket_author_id"`
	Round           *int     `json:"round"`
	CurrentRound    *int     `json:"current_round"`
	Votes           int64    `json:"votes"`
	Likes           int64    `json:"likes"`
	Comments        int64    `json:"comments"`
	EngagementScore float64  `json:"engagement_score"`
	Rank            int64    `json:"rank"`
}

type PopularBracketDTO struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	AuthorID        string  `json:"author_id"`
	CurrentRound    int     `json:"current_round"`
	Size            int     `json:"size"`
	Votes           int64   `json:"votes"`
	Likes           int64   `json:"likes"`
	Comments        int64   `json:"comments"`
	EngagementScore float64 `json:"engagement_score"`
	Rank            int64   `json:"rank"`
}

type HomeCreatorDTO struct {
	ID               string `json:"id"`
	Username         string `json:"username"`
	AvatarPath       string `json:"avatar_path"`
	FollowersCount   int    `json:"followers_count"`
	FollowingCount   int    `json:"following_count"`
	IsPrivate        bool   `json:"is_private"`
	ViewerFollowing  bool   `json:"viewer_following"`
	ViewerFollowedBy bool   `json:"viewer_followed_by"`
	Mutual           bool   `json:"mutual"`
}

type HomeMatchupDTO struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	AuthorID  string     `json:"author_id"`
	BracketID *string    `json:"bracket_id"`
	CreatedAt time.Time  `json:"created_at"`
}

type AdminMatchupDTO struct {
	ID             string           `json:"id"`
	Title          string           `json:"title"`
	Content        string           `json:"content"`
	AuthorID       string           `json:"author_id"`
	AuthorUsername string           `json:"author_username"`
	Items          []MatchupItemDTO `json:"items"`
	LikesCount     int64            `json:"likes_count"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

type AdminBracketDTO struct {
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	AuthorID       string    `json:"author_id"`
	AuthorUsername string    `json:"author_username"`
	Status         string    `json:"status"`
	CurrentRound   int       `json:"current_round"`
	LikesCount     int64     `json:"likes_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type BracketSummaryDTO struct {
	Bracket       BracketDTO   `json:"bracket"`
	Matchups      []MatchupDTO `json:"matchups"`
	LikedMatchups []string     `json:"liked_matchup_ids"`
	LikedBracket  bool         `json:"liked_bracket"`
}
