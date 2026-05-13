package controllers

import (
	"context"

	"Matchup/models"
	httpctx "Matchup/utils/httpctx"

	"github.com/jmoiron/sqlx"
)

const (
	visibilityPublic        = "public"
	visibilityFollowers     = "followers"
	visibilityMutuals       = "mutuals"
	// visibilityCommunityOnly is the default for community-scoped
	// matchups + brackets. Visible to non-banned members only;
	// non-members see the community page without the item, and
	// direct-URL access returns PermissionDenied. Set by the
	// creator when the matchup/bracket is community-scoped; the
	// alternative is `public` which lets any viewer see (but
	// vote-gating still requires membership).
	visibilityCommunityOnly = "community-only"
)

// normalizeVisibility coerces a client-supplied string into one of the
// known modes. Unknown values fall back to public — fail-open so an
// upgrade-skew client doesn't silently lock itself out of its own post.
func normalizeVisibility(value string) string {
	switch value {
	case visibilityMutuals:
		return visibilityMutuals
	case visibilityFollowers:
		return visibilityFollowers
	case visibilityCommunityOnly:
		return visibilityCommunityOnly
	default:
		return visibilityPublic
	}
}

// optionalViewerFromCtx returns the viewer ID from context (set by SoftJWTMiddleware or TokenAuthMiddleware).
func optionalViewerFromCtx(ctx context.Context) (uint, bool) {
	return httpctx.CurrentUserID(ctx)
}

func isFollower(db sqlx.ExtContext, followerID, followedID uint) (bool, error) {
	var count int64
	err := sqlx.GetContext(context.Background(), db, &count,
		"SELECT COUNT(*) FROM follows WHERE follower_id = $1 AND followed_id = $2",
		followerID, followedID)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func isMutual(db sqlx.ExtContext, aID, bID uint) (bool, error) {
	fwd, err := isFollower(db, aID, bID)
	if err != nil || !fwd {
		return false, err
	}
	rev, err := isFollower(db, bID, aID)
	if err != nil {
		return false, err
	}
	return rev, nil
}

func canViewUserContent(db sqlx.ExtContext, viewerID uint, hasViewer bool, owner *models.User, visibility string, isAdmin bool) (bool, string, error) {
	if owner == nil {
		return false, "followers", nil
	}
	if isAdmin || (hasViewer && viewerID == owner.ID) {
		return true, "", nil
	}

	normalized := normalizeVisibility(visibility)
	// Followers-only: viewer must follow the owner. Strictly weaker
	// than mutuals (one-way is enough). Anon viewers fail immediately
	// since they can't follow anything.
	if normalized == visibilityFollowers {
		if !hasViewer {
			return false, "followers", nil
		}
		follower, err := isFollower(db, viewerID, owner.ID)
		if err != nil {
			return false, "", err
		}
		if !follower {
			return false, "followers", nil
		}
	}
	if normalized == visibilityMutuals {
		if !hasViewer {
			return false, visibilityMutuals, nil
		}
		mutual, err := isMutual(db, viewerID, owner.ID)
		if err != nil {
			return false, "", err
		}
		if !mutual {
			return false, visibilityMutuals, nil
		}
	}

	if owner.IsPrivate {
		if !hasViewer {
			return false, "followers", nil
		}
		follower, err := isFollower(db, viewerID, owner.ID)
		if err != nil {
			return false, "", err
		}
		if !follower {
			return false, "followers", nil
		}
	}

	return true, "", nil
}

func visibilityErrorMessage(reason string) string {
	switch reason {
	case visibilityMutuals:
		return "This content is only visible to mutual followers"
	case visibilityCommunityOnly:
		return "Join this community to view"
	default:
		return "This content is only visible to followers"
	}
}

// resolveCommunityVisibility applies the visibility rules for
// community-scoped content at create time. Returns the validated
// visibility string or a sentinel error. Standalone content (no
// community_id) bypasses this — the caller uses normalizeVisibility
// directly.
//
//   - Community-scoped matchups/brackets accept only 'community-only'
//     (default) and 'public'. Followers / mutuals make no sense for
//     content owned by a community rather than a person, so we reject
//     them with InvalidArgument rather than silently coercing.
func resolveCommunityVisibility(raw string) (string, bool) {
	switch raw {
	case "", visibilityCommunityOnly:
		return visibilityCommunityOnly, true
	case visibilityPublic:
		return visibilityPublic, true
	default:
		return "", false
	}
}

// canViewCommunityContent gates access to a community-scoped matchup
// or bracket. Returns (allowed, reason, error).
//
//   - public visibility: anyone can view.
//   - community-only: viewer must have a non-banned membership row
//     for this community. Anon viewers always fail.
//   - Owner of the matchup/bracket bypasses the check (they're always
//     a member of their own community — CreateMatchup gates on it).
func canViewCommunityContent(db sqlx.ExtContext, viewerID uint, hasViewer bool, communityID uint, visibility string) (bool, string, error) {
	if normalizeVisibility(visibility) != visibilityCommunityOnly {
		return true, "", nil
	}
	if !hasViewer {
		return false, visibilityCommunityOnly, nil
	}
	var role string
	err := sqlx.GetContext(context.Background(), db, &role,
		"SELECT role FROM community_memberships WHERE community_id = $1 AND user_id = $2",
		communityID, viewerID)
	if err != nil {
		// Treat "no row" as not-a-member. Other errors propagate.
		return false, visibilityCommunityOnly, nil
	}
	if role == "" || role == "banned" {
		return false, visibilityCommunityOnly, nil
	}
	return true, "", nil
}
