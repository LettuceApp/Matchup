package controllers

import (
	"net/http"

	"Matchup/auth"
	"Matchup/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	visibilityPublic  = "public"
	visibilityMutuals = "mutuals"
)

func normalizeVisibility(value string) string {
	switch value {
	case visibilityMutuals:
		return visibilityMutuals
	default:
		return visibilityPublic
	}
}

func optionalViewerID(c *gin.Context) (uint, bool) {
	uid, err := auth.ExtractTokenID(c.Request)
	if err != nil {
		return 0, false
	}
	return uid, true
}

func isFollower(db *gorm.DB, followerID, followedID uint) (bool, error) {
	var count int64
	if err := db.Model(&models.Follow{}).
		Where("follower_id = ? AND followed_id = ?", followerID, followedID).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func isMutual(db *gorm.DB, aID, bID uint) (bool, error) {
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

func canViewUserContent(db *gorm.DB, viewerID uint, hasViewer bool, owner *models.User, visibility string, isAdmin bool) (bool, string, error) {
	if owner == nil {
		return false, "followers", nil
	}
	if isAdmin || (hasViewer && viewerID == owner.ID) {
		return true, "", nil
	}

	normalized := normalizeVisibility(visibility)
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

func respondVisibilityDenied(c *gin.Context, reason string) {
	c.JSON(http.StatusForbidden, gin.H{"error": visibilityErrorMessage(reason)})
}
