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
//   - "@me" / "me"            → resolves to the requesting user (viewerID)
//   - "@username" / "username" → resolved by case-insensitive username
//   - a user public_id (UUID)  → resolved by public_id
//
// Trims leading @ + whitespace. The "me" sentinel is the social-loop
// cycle's convention for self-mention — lets a creator drop
// themselves into their own matchup without typing their username out.
// Rejects deleted/banned users — they can't be added as a matchup
// item even if their old handle still resolves. The viewerID
// parameter is required so "me" can resolve; pass 0 for unauthed
// contexts (in which case "me" returns an error).
func resolveUserHandle(ctx context.Context, db sqlx.ExtContext, handle string, viewerID uint) (*models.User, error) {
	clean := strings.TrimSpace(handle)
	clean = strings.TrimPrefix(clean, "@")
	if clean == "" {
		return nil, errors.New("empty user handle")
	}

	// "me" sentinel — explicit self-mention. Case-insensitive so
	// `@Me` and `@ME` work too. Requires an authed caller; an anon
	// resolver would have no "me" to resolve to.
	if strings.EqualFold(clean, "me") {
		if viewerID == 0 {
			return nil, errors.New("sign in to use @me")
		}
		var u models.User
		if err := sqlx.GetContext(ctx, db, &u, `
			SELECT * FROM users
			WHERE id = $1
			  AND deleted_at IS NULL
			  AND banned_at IS NULL
		`, viewerID); err != nil {
			return nil, errors.New("could not resolve @me")
		}
		return &u, nil
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

// validateMatchupItemUser checks that a referenced user can be added
// as a matchup item.
//
// Phase 2 of the social-loop cycle originally gated this strictly —
// mutual on standalone, member on community. The constraint was
// dropped to maximize shareability: putting a friend's friend, a
// public-figure-you-follow, or yourself in a matchup should "just
// work" so a creator's first instinct ("can I tag X?") doesn't hit
// a wall. Backstop: deleted + banned users are already filtered out
// by resolveUserHandle, and the recipient's notification preferences
// still gate whether they hear about it. Abuse can be addressed
// downstream via block / report if it becomes a problem.
//
// The function still exists so future tightening (community-only,
// follower-only) can be re-introduced behind a feature flag without
// changing CreateMatchup's call shape. Today it's effectively a
// nil-check.
func validateMatchupItemUser(_ context.Context, _ sqlx.ExtContext, _ uint, _ *uint, targetID uint) error {
	if targetID == 0 {
		return errors.New("invalid user reference")
	}
	return nil
}
