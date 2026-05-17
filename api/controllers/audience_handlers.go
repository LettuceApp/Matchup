package controllers

// audience_handlers.go implements the owner-only "audience" surface
// — the panels on a matchup or bracket detail page that let the
// creator see who voted and who liked their content. Three RPCs
// land here, kept together because they share auth posture, query
// shape, and proto-mapper helpers:
//
//	MatchupService.GetMatchupVoters — voter list w/ pick + anon count
//	MatchupService.GetMatchupLikers — liker username list
//	BracketService.GetBracketLikers — bracket-level liker list
//
// Auth posture: strictly owner-only. Admins do NOT bypass — these
// are author-experience controls, not moderation surfaces, and the
// project's memory note explicitly forbids `|| isAdmin` escapes on
// owner-gated public-page features.

import (
	"context"
	"database/sql"
	"errors"

	bracketv1 "Matchup/gen/bracket/v1"
	commonv1 "Matchup/gen/common/v1"
	matchupv1 "Matchup/gen/matchup/v1"
	"Matchup/models"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
)

// ---- matchup voters --------------------------------------------------

// GetMatchupVoters returns the matchup author's audience. Each named
// (signed-in) voter comes back with their current pick label; anon
// voters are collapsed into a single count by explicit product
// decision — a 50-row list of grey silhouettes is noise, the count
// alone is the useful signal.
func (h *MatchupHandler) GetMatchupVoters(ctx context.Context, req *connect.Request[matchupv1.GetMatchupVotersRequest]) (*connect.Response[matchupv1.GetMatchupVotersResponse], error) {
	userID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	matchupRecord, err := resolveMatchupByIdentifier(h.DB, req.Msg.GetMatchupId())
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		} else if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("matchup not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if matchupRecord.AuthorID != userID {
		// Strict owner-only — admins included. Voter identities are
		// audience data, not moderation data.
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("owner only"))
	}

	// Named voters joined to users + the picked item's label. Order
	// by created_at DESC so the most recently engaged faces sit at
	// the top of the panel (which is what owners care about most).
	// kind = 'pick' filters out 'skip' rows (which have a NULL
	// item_public_id) without us needing to LEFT JOIN.
	type voterRow struct {
		PublicID    string         `db:"public_id"`
		Username    string         `db:"username"`
		AvatarPath  sql.NullString `db:"avatar_path"`
		PickedLabel sql.NullString `db:"item"`
	}
	var rows []voterRow
	if err := sqlx.SelectContext(ctx, h.DB, &rows, `
		SELECT u.public_id, u.username, u.avatar_path, mi.item
		FROM matchup_votes mv
		JOIN users u ON u.id = mv.user_id
		LEFT JOIN matchup_items mi ON mi.public_id = mv.matchup_item_public_id
		WHERE mv.matchup_public_id = $1
		  AND mv.kind = 'pick'
		  AND mv.user_id IS NOT NULL
		  AND u.deleted_at IS NULL
		  AND u.banned_at IS NULL
		ORDER BY mv.created_at DESC
	`, matchupRecord.PublicID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	voters := make([]*matchupv1.MatchupVoter, 0, len(rows))
	for _, r := range rows {
		avatar := ""
		if r.AvatarPath.Valid {
			// Resolve to full S3 URL like every other surface that
			// embeds UserSummaryResponse — the frontend renders
			// avatar_path as <img src> directly without rebuilding.
			u := models.User{AvatarPath: r.AvatarPath.String}
			u.ProcessAvatarPath()
			avatar = u.AvatarPath
		}
		label := ""
		if r.PickedLabel.Valid {
			label = r.PickedLabel.String
		}
		voters = append(voters, &matchupv1.MatchupVoter{
			User: &commonv1.UserSummaryResponse{
				Id:         r.PublicID,
				Username:   r.Username,
				AvatarPath: avatar,
			},
			PickedItemLabel: label,
		})
	}

	// Anon voter count — same WHERE clause, just user_id IS NULL.
	// Counts only 'pick' rows; anon skips don't represent an
	// engagement signal worth surfacing here.
	var anonCount int32
	if err := sqlx.GetContext(ctx, h.DB, &anonCount, `
		SELECT COUNT(*)
		FROM matchup_votes
		WHERE matchup_public_id = $1
		  AND kind = 'pick'
		  AND user_id IS NULL
	`, matchupRecord.PublicID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&matchupv1.GetMatchupVotersResponse{
		Voters:    voters,
		AnonCount: anonCount,
	}), nil
}

// ---- matchup likers --------------------------------------------------

func (h *MatchupHandler) GetMatchupLikers(ctx context.Context, req *connect.Request[matchupv1.GetMatchupLikersRequest]) (*connect.Response[matchupv1.GetMatchupLikersResponse], error) {
	userID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	matchupRecord, err := resolveMatchupByIdentifier(h.DB, req.Msg.GetMatchupId())
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		} else if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("matchup not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if matchupRecord.AuthorID != userID {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("owner only"))
	}

	likers, err := loadLikersForMatchup(ctx, h.DB, matchupRecord.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&matchupv1.GetMatchupLikersResponse{Likers: likers}), nil
}

// ---- bracket likers --------------------------------------------------

func (h *BracketHandler) GetBracketLikers(ctx context.Context, req *connect.Request[bracketv1.GetBracketLikersRequest]) (*connect.Response[bracketv1.GetBracketLikersResponse], error) {
	userID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	bracketRecord, err := resolveBracketByIdentifier(h.DB, req.Msg.GetBracketId())
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		} else if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if bracketRecord.AuthorID != userID {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("owner only"))
	}

	likers, err := loadLikersForBracket(ctx, h.DB, bracketRecord.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&bracketv1.GetBracketLikersResponse{Likers: likers}), nil
}

// ---- helpers ---------------------------------------------------------

// likerRow is the shared scan target for both liker helpers below.
// Lives at package scope so the two helpers can pass `[]likerRow`
// to a single mapper without Go's named-type-equality rules
// complaining about an inline struct in each call site.
type likerRow struct {
	PublicID   string         `db:"public_id"`
	Username   string         `db:"username"`
	AvatarPath sql.NullString `db:"avatar_path"`
}

// loadLikersForMatchup returns the named users who liked this matchup,
// most recent first, with avatar URLs resolved to CDN absolutes.
// Deleted + banned users are excluded — a like from a since-deleted
// account isn't an audience signal worth surfacing.
func loadLikersForMatchup(ctx context.Context, db *sqlx.DB, matchupID uint) ([]*commonv1.UserSummaryResponse, error) {
	var rows []likerRow
	if err := sqlx.SelectContext(ctx, db, &rows, `
		SELECT u.public_id, u.username, u.avatar_path
		FROM likes l
		JOIN users u ON u.id = l.user_id
		WHERE l.matchup_id = $1
		  AND u.deleted_at IS NULL
		  AND u.banned_at IS NULL
		ORDER BY l.created_at DESC
	`, matchupID); err != nil {
		return nil, err
	}
	return likerRowsToUserSummaries(rows), nil
}

// loadLikersForBracket is the same shape as loadLikersForMatchup but
// against the bracket_likes table.
func loadLikersForBracket(ctx context.Context, db *sqlx.DB, bracketID uint) ([]*commonv1.UserSummaryResponse, error) {
	var rows []likerRow
	if err := sqlx.SelectContext(ctx, db, &rows, `
		SELECT u.public_id, u.username, u.avatar_path
		FROM bracket_likes bl
		JOIN users u ON u.id = bl.user_id
		WHERE bl.bracket_id = $1
		  AND u.deleted_at IS NULL
		  AND u.banned_at IS NULL
		ORDER BY bl.created_at DESC
	`, bracketID); err != nil {
		return nil, err
	}
	return likerRowsToUserSummaries(rows), nil
}

// likerRowsToUserSummaries — small mapper to keep the two helpers
// from duplicating the avatar-resolution branch.
func likerRowsToUserSummaries(rows []likerRow) []*commonv1.UserSummaryResponse {
	out := make([]*commonv1.UserSummaryResponse, 0, len(rows))
	for _, r := range rows {
		avatar := ""
		if r.AvatarPath.Valid {
			u := models.User{AvatarPath: r.AvatarPath.String}
			u.ProcessAvatarPath()
			avatar = u.AvatarPath
		}
		out = append(out, &commonv1.UserSummaryResponse{
			Id:         r.PublicID,
			Username:   r.Username,
			AvatarPath: avatar,
		})
	}
	return out
}
