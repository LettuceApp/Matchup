package controllers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"Matchup/models"

	"github.com/jmoiron/sqlx"
)

// resolveUserHandle turns a free-form @-handle into a User row. Accepts:
//   - "@username"
//   - "username"
//   - a user public_id (UUID)
//
// Trims leading @ + whitespace. Looks up by public_id OR by
// case-insensitive username in one query so the caller doesn't have to
// guess. Rejects deleted/banned users — they can't be added as a
// matchup item even if their old handle still resolves.
func resolveUserHandle(ctx context.Context, db sqlx.ExtContext, handle string) (*models.User, error) {
	clean := strings.TrimSpace(handle)
	clean = strings.TrimPrefix(clean, "@")
	if clean == "" {
		return nil, errors.New("empty user handle")
	}
	var u models.User
	err := sqlx.GetContext(ctx, db, &u, `
		SELECT *
		FROM users
		WHERE (public_id = $1 OR LOWER(username) = LOWER($1))
		  AND deleted_at IS NULL
		  AND banned_at IS NULL
		LIMIT 1
	`, clean)
	if err != nil {
		return nil, fmt.Errorf("user %q not found", clean)
	}
	return &u, nil
}

// validateMatchupItemUser enforces the social-loop rule that decides
// who can be added as a matchup item.
//
//   - Self-mention: always rejected. A creator can't put themselves as
//     a contender in their own matchup.
//   - Community matchup (communityID non-nil): target must be a non-
//     banned member of the same community as the creator. Banned
//     members + non-members are rejected; the creator is presumed
//     already validated as a member elsewhere.
//   - Standalone matchup (communityID nil): target must be a mutual
//     of the creator — they follow each other both ways. Personal
//     matchups stay limited to the creator's close network so a
//     stranger can't be dragged into one.
//
// On rejection, the returned error message is safe to surface to the
// client — it doesn't leak the existence of the target user beyond
// what their public profile already does.
func validateMatchupItemUser(ctx context.Context, db sqlx.ExtContext, creatorID uint, communityID *uint, targetID uint) error {
	if creatorID == targetID {
		return errors.New("you can't add yourself as a matchup item")
	}
	if communityID != nil {
		// Community matchup: must share a non-banned membership in this
		// community. We don't separately re-check the creator's
		// membership here because CreateMatchup already gated on it.
		var role string
		err := sqlx.GetContext(ctx, db, &role,
			"SELECT role FROM community_memberships WHERE community_id = $1 AND user_id = $2",
			*communityID, targetID,
		)
		if err != nil || role == "" || role == "banned" {
			return errors.New("user must be a member of this community to be added")
		}
		return nil
	}
	// Standalone matchup: mutual edge required.
	var count int
	err := sqlx.GetContext(ctx, db, &count, `
		SELECT COUNT(*)
		FROM follows f1
		JOIN follows f2
		  ON f2.followed_id = f1.follower_id
		 AND f2.follower_id = f1.followed_id
		WHERE f1.follower_id = $1 AND f1.followed_id = $2
	`, creatorID, targetID)
	if err != nil || count == 0 {
		return errors.New("user must be a mutual (follow each other) to be added")
	}
	return nil
}
