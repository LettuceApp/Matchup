package controllers

import (
	"context"

	"Matchup/models"
	httpctx "Matchup/utils/httpctx"

	"github.com/jmoiron/sqlx"
)

const (
	visibilityPublic    = "public"
	visibilityFollowers = "followers"
	visibilityMutuals   = "mutuals"
)

// normalizeVisibility coerces a client-supplied string into one of the
// three known modes. Anything we don't recognise falls back to public,
// which is the fail-open default — better that an upgrade-skew client
// sees a public matchup than silently locks itself out of its own post.
func normalizeVisibility(value string) string {
	switch value {
	case visibilityMutuals:
		return visibilityMutuals
	case visibilityFollowers:
		return visibilityFollowers
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
	if reason == visibilityMutuals {
		return "This content is only visible to mutual followers"
	}
	return "This content is only visible to followers"
}
