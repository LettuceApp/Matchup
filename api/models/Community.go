package models

import (
	"time"

	"github.com/lib/pq"
)

// Community is the v1 communities-feature row. Standalone matchups
// keep working unchanged (matchups.community_id stays nullable); a
// community is only attached when the matchup is created from within
// a community context.
//
// Counts (member_count, matchup_count, bracket_count) are denormalized
// so community-directory cards don't fan out N COUNT(*) queries. They
// are incremented / decremented inline by the handlers that own the
// write path — join/leave for memberships, attach/detach for content.
type Community struct {
	ID           uint           `db:"id" json:"id"`
	PublicID     string         `db:"public_id" json:"public_id"`
	Slug         string         `db:"slug" json:"slug"`
	Name         string         `db:"name" json:"name"`
	Description  string         `db:"description" json:"description"`
	AvatarPath   string         `db:"avatar_path" json:"avatar_path"`
	BannerPath   string         `db:"banner_path" json:"banner_path"`
	Tags         pq.StringArray `db:"tags" json:"tags"`
	Privacy      string         `db:"privacy" json:"privacy"`
	// ThemeGradient is a curated-palette slug (e.g. 'stardust' /
	// 'sunset'). Empty = no theme chosen → frontend falls back to the
	// default stardust palette. Stored as a slug, not raw CSS, so we
	// can iterate on the palette without a migration.
	ThemeGradient string         `db:"theme_gradient" json:"theme_gradient"`
	OwnerID      uint           `db:"owner_id" json:"owner_id"`
	MemberCount  int64          `db:"member_count" json:"member_count"`
	MatchupCount int64          `db:"matchup_count" json:"matchup_count"`
	BracketCount int64          `db:"bracket_count" json:"bracket_count"`
	CreatedAt    time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time      `db:"updated_at" json:"updated_at"`
	DeletedAt    *time.Time     `db:"deleted_at" json:"-"`
}

// CommunityMembership represents one (community, user) edge. Owners
// have a row here too — created at community-create time with role
// 'owner'. A banned user keeps a row with role='banned' rather than
// being deleted, so a re-join attempt is rejected by the unique
// constraint without needing an extra blocklist table.
type CommunityMembership struct {
	ID               uint       `db:"id" json:"id"`
	CommunityID      uint       `db:"community_id" json:"community_id"`
	UserID           uint       `db:"user_id" json:"user_id"`
	Role             string     `db:"role" json:"role"`
	JoinedAt         time.Time  `db:"joined_at" json:"joined_at"`
	InvitedByUserID  *uint      `db:"invited_by_user_id" json:"invited_by_user_id"`
	BannedAt         *time.Time `db:"banned_at" json:"banned_at"`
	BannedByUserID   *uint      `db:"banned_by_user_id" json:"banned_by_user_id"`
	BannedReason     *string    `db:"banned_reason" json:"banned_reason"`
}

// CommunityRule is a single numbered rule shown on the community's
// About page. SetRules wipes + re-inserts as a full replacement so
// ordering matches the order client sends.
type CommunityRule struct {
	ID          uint      `db:"id" json:"id"`
	CommunityID uint      `db:"community_id" json:"community_id"`
	Position    int32     `db:"position" json:"position"`
	Title       string    `db:"title" json:"title"`
	Body        string    `db:"body" json:"body"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}
