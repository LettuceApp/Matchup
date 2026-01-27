package controllers

// MatchupCreateRequest describes a minimal payload for creating a matchup.
type MatchupCreateRequest struct {
	Title           string              `json:"title"`
	Content         string              `json:"content,omitempty"`
	Items           []MatchupItemCreate `json:"items,omitempty"`
	EndMode         string              `json:"end_mode,omitempty"`
	DurationSeconds int                 `json:"duration_seconds,omitempty"`
	BracketID       *string             `json:"bracket_id,omitempty"`
	Round           *int                `json:"round,omitempty"`
	Seed            *int                `json:"seed,omitempty"`
}

type MatchupItemCreate struct {
	Item string `json:"item"`
}

type MatchupItemResponse struct {
	ID    string `json:"id"`
	Item  string `json:"item"`
	Votes int    `json:"votes"`
}

type MatchupResponse struct {
	ID         string                `json:"id"`
	Title      string                `json:"title"`
	Content    string                `json:"content"`
	Status     string                `json:"status"`
	AuthorID   string                `json:"author_id"`
	BracketID  *string               `json:"bracket_id,omitempty"`
	Round      *int                  `json:"round,omitempty"`
	Seed       *int                  `json:"seed,omitempty"`
	LikesCount int64                 `json:"likes_count"`
	Items      []MatchupItemResponse `json:"items"`
}

type PaginationResponse struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

type MatchupResponseEnvelope struct {
	Status   int             `json:"status"`
	Response MatchupResponse `json:"response"`
}

type MatchupListResponse struct {
	Status     int                `json:"status"`
	Response   []MatchupResponse  `json:"response"`
	Pagination PaginationResponse `json:"pagination"`
}

type ErrorResponse struct {
	Error interface{} `json:"error"`
}

type SimpleMessageResponse struct {
	Status   int         `json:"status"`
	Response interface{} `json:"response"`
}

type UserResponse struct {
	ID             string `json:"id"`
	Username       string `json:"username"`
	Email          string `json:"email"`
	AvatarPath     string `json:"avatar_path"`
	IsAdmin        bool   `json:"is_admin"`
	IsPrivate      bool   `json:"is_private"`
	FollowersCount int64  `json:"followers_count"`
	FollowingCount int64  `json:"following_count"`
}

type UserResponseEnvelope struct {
	Status   int          `json:"status"`
	Response UserResponse `json:"response"`
}

type UserCreateRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserUpdateRequest struct {
	Email           string `json:"email,omitempty"`
	CurrentPassword string `json:"current_password,omitempty"`
	NewPassword     string `json:"new_password,omitempty"`
}

type UserPrivacyUpdateRequest struct {
	IsPrivate bool `json:"is_private"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token      string `json:"token"`
	ID         string `json:"id"`
	Email      string `json:"email"`
	Username   string `json:"username"`
	IsAdmin    bool   `json:"is_admin"`
	AvatarPath string `json:"avatar_path"`
	IsPrivate  bool   `json:"is_private"`
}

type LoginResponseEnvelope struct {
	Status   int           `json:"status"`
	Response LoginResponse `json:"response"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

type ResetPasswordRequest struct {
	NewPassword    string `json:"new_password"`
	RetypePassword string `json:"retype_password"`
	Token          string `json:"token"`
}

type UsersListResponse struct {
	Status   int            `json:"status"`
	Response []UserResponse `json:"response"`
}

type CommentCreateRequest struct {
	Body string `json:"body"`
}

type CommentUpdateRequest struct {
	Body string `json:"body"`
}

type CommentResponseDoc struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type CommentListResponseDoc struct {
	Status   int                  `json:"status"`
	Response []CommentResponseDoc `json:"response"`
}

type MatchupItemCreateRequest struct {
	Item string `json:"item"`
}

type MatchupItemUpdateRequest struct {
	Item string `json:"item"`
}

type MatchupItemResponseEnvelope struct {
	Status   int                 `json:"status"`
	Response MatchupItemResponse `json:"response"`
}

type MatchupItemDeleteResponse struct {
	Message string `json:"message"`
}

type UpdateMatchupRequest struct {
	Title   string `json:"title,omitempty"`
	Content string `json:"content,omitempty"`
}

type UpdateBracketRequest struct {
	Title                *string `json:"title,omitempty"`
	Description          *string `json:"description,omitempty"`
	Status               *string `json:"status,omitempty"`
	AdvanceMode          *string `json:"advance_mode,omitempty"`
	DurationMinutes      *int    `json:"duration_minutes,omitempty"`
	RoundDurationSeconds *int    `json:"round_duration_seconds,omitempty"`
}

type AttachMatchupRequest struct {
	MatchupID string `json:"matchup_id"`
	Round     int    `json:"round"`
	Seed      *int   `json:"seed,omitempty"`
}

type PopularMatchupsEnvelope struct {
	Status   string             `json:"status"`
	Response []PopularMatchupDTO `json:"response"`
}

type PopularBracketsEnvelope struct {
	Status   string              `json:"status"`
	Response []PopularBracketDTO `json:"response"`
}

type HomeSummaryEnvelope struct {
	Status   string      `json:"status"`
	Response HomeSummary `json:"response"`
}

type LikeResponseDoc struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	MatchupID string `json:"matchup_id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type BracketLikeResponseDoc struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	BracketID string `json:"bracket_id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type MatchupVoteResponseDoc struct {
	ID            string  `json:"id"`
	UserID        *string `json:"user_id"`
	MatchupID     string  `json:"matchup_id"`
	MatchupItemID string  `json:"matchup_item_id"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type RelationshipResponse struct {
	Following  bool `json:"following"`
	FollowedBy bool `json:"followed_by"`
	Mutual     bool `json:"mutual"`
}

type FollowListUser struct {
	ID               string `json:"id"`
	Username         string `json:"username"`
	Email            string `json:"email"`
	AvatarPath       string `json:"avatar_path"`
	IsAdmin          bool   `json:"is_admin"`
	FollowersCount   int64  `json:"followers_count"`
	FollowingCount   int64  `json:"following_count"`
	ViewerFollowing  bool   `json:"viewer_following,omitempty"`
	ViewerFollowedBy bool   `json:"viewer_followed_by,omitempty"`
	Mutual           bool   `json:"mutual,omitempty"`
}

type FollowListPayload struct {
	Users      []FollowListUser `json:"users"`
	NextCursor *string          `json:"next_cursor,omitempty"`
}

type FollowListResponse struct {
	Status   int               `json:"status"`
	Response FollowListPayload `json:"response"`
}

type LikesListResponse struct {
	Status   int               `json:"status"`
	Response []LikeResponseDoc `json:"response"`
}

type BracketLikesListResponse struct {
	Status   int                      `json:"status"`
	Response []BracketLikeResponseDoc `json:"response"`
}

type MatchupVotesListResponse struct {
	Status   int                      `json:"status"`
	Response []MatchupVoteResponseDoc `json:"response"`
}

type UserMatchupsResponse struct {
	Status     int                `json:"status"`
	Response   []MatchupResponse  `json:"response"`
	Pagination PaginationResponse `json:"pagination"`
}

type UserBracketsResponse struct {
	Response []BracketResponse `json:"response"`
}

type BracketMatchupsResponse struct {
	Response []MatchupResponse `json:"response"`
}

type BracketCreateRequest struct {
	Title                string   `json:"title"`
	Description          string   `json:"description,omitempty"`
	Size                 int      `json:"size"`
	AdvanceMode          string   `json:"advance_mode,omitempty"`
	DurationMinutes      *int     `json:"duration_minutes,omitempty"`
	RoundDurationSeconds *int     `json:"round_duration_seconds,omitempty"`
	Visibility           string   `json:"visibility,omitempty"`
	Entries              []string `json:"entries,omitempty"`
}

type BracketResponse struct {
	ID                   string `json:"id"`
	Title                string  `json:"title"`
	Description          string  `json:"description"`
	AuthorID             string  `json:"author_id"`
	Size                 int     `json:"size"`
	Status               string  `json:"status"`
	CurrentRound         int     `json:"current_round"`
	AdvanceMode          string  `json:"advance_mode"`
	RoundDurationSeconds int     `json:"round_duration_seconds"`
	RoundStartedAt       *string `json:"round_started_at,omitempty"`
	RoundEndsAt          *string `json:"round_ends_at,omitempty"`
	LikesCount           int64   `json:"likes_count"`
}

type BracketResponseEnvelope struct {
	Response BracketResponse `json:"response"`
}

type BracketSummaryResponse struct {
	Bracket       BracketResponse   `json:"bracket"`
	Matchups      []MatchupResponse `json:"matchups"`
	LikedMatchups []string          `json:"liked_matchup_ids"`
	LikedBracket  bool              `json:"liked_bracket"`
}

type BracketSummaryEnvelope struct {
	Status   string                 `json:"status"`
	Response BracketSummaryResponse `json:"response"`
}

type MatchupItemVoteResponse struct {
	Status       int                 `json:"status"`
	Response     MatchupItemResponse `json:"response"`
	AlreadyVoted bool                `json:"already_voted,omitempty"`
}

type WinnerOverrideResponse struct {
	Message      string `json:"message"`
	WinnerItemID string `json:"winner_item_id"`
}

type WinnerOverrideRequest struct {
	WinnerItemID string `json:"winner_item_id"`
}

type CompleteMatchupResponse struct {
	Message      string `json:"message"`
	Status       string `json:"status"`
	WinnerItemID string `json:"winner_item_id"`
}

type BracketCommentCreateRequest struct {
	Body string `json:"body"`
}

type BracketCommentResponseDoc struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type BracketCommentListResponseDoc struct {
	Status   int                         `json:"status"`
	Response []BracketCommentResponseDoc `json:"response"`
}
