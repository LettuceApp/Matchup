package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	searchv1 "Matchup/gen/search/v1"
	"Matchup/gen/search/v1/searchv1connect"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// SearchHandler implements the universal-search RPC. Each entity
// (matchup / bracket / community / user) is queried separately
// against the same normalized query string; the response packs all
// four lists so the frontend can render Twitter-style tabs without a
// second round-trip per tab.
type SearchHandler struct {
	DB     *sqlx.DB
	ReadDB *sqlx.DB
}

var _ searchv1connect.SearchServiceHandler = (*SearchHandler)(nil)

const (
	minSearchQueryLen   = 2
	defaultSearchLimit  = 10
	maxSearchLimit      = 50
	searchTypeMatchup   = "matchup"
	searchTypeBracket   = "bracket"
	searchTypeCommunity = "community"
	searchTypeUser      = "user"
)

func (h *SearchHandler) Search(ctx context.Context, req *connect.Request[searchv1.SearchRequest]) (*connect.Response[searchv1.SearchResponse], error) {
	q := strings.ToLower(strings.TrimSpace(req.Msg.GetQuery()))
	if len(q) < minSearchQueryLen {
		// Empty / single-char queries return an empty response. Lets
		// the dropdown autocomplete render "type more to search…"
		// without hammering the DB on every keystroke.
		return connect.NewResponse(&searchv1.SearchResponse{}), nil
	}
	limit := int(req.Msg.GetLimit())
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}
	only := strings.ToLower(strings.TrimSpace(req.Msg.GetOnly()))

	resp := &searchv1.SearchResponse{}
	db := dbForRead(ctx, h.DB, h.ReadDB)
	pattern := "%" + q + "%"

	viewerID, hasViewer := httpctx.CurrentUserID(ctx)

	if only == "" || only == searchTypeMatchup {
		ms, err := searchMatchups(ctx, db, pattern, limit, viewerID, hasViewer)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		resp.Matchups = ms
	}
	if only == "" || only == searchTypeBracket {
		bs, err := searchBrackets(ctx, db, pattern, limit, viewerID, hasViewer)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		resp.Brackets = bs
	}
	if only == "" || only == searchTypeCommunity {
		cs, err := searchCommunities(ctx, db, pattern, limit)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		resp.Communities = cs
	}
	if only == "" || only == searchTypeUser {
		us, err := searchUsers(ctx, db, pattern, limit)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		resp.Users = us
	}

	return connect.NewResponse(resp), nil
}

// ------------ matchups ------------
//
// Visibility filter:
//   - Only visibility='public' surfaces. Community-only items NEVER
//     show up in search (they live exclusively in the community
//     feed). Followers-only / mutuals-only items don't surface here
//     either — Twitter posture: protected content needs to be found
//     via the author's profile, not search.
//   - Bracket-child matchups (per-round rows auto-generated inside a
//     bracket) are filtered out — they're not standalone content.
//   - Soft-deleted communities + authors are filtered out.
func searchMatchups(ctx context.Context, db sqlx.ExtContext, pattern string, limit int, viewerID uint, hasViewer bool) ([]*searchv1.SearchMatchup, error) {
	// Viewer info reserved for follow-aware visibility expansion
	// later (e.g. allowing followers-only matchups to appear for the
	// follower). Today we keep search public-only for simplicity.
	_ = viewerID
	_ = hasViewer

	type row struct {
		PublicID       string         `db:"public_id"`
		ShortID        *string        `db:"short_id"`
		Title          string         `db:"title"`
		AuthorPublicID string         `db:"author_public_id"`
		AuthorUsername string         `db:"author_username"`
		CreatedAt      time.Time      `db:"created_at"`
		LikesCount     int            `db:"likes_count"`
		CommentsCount  int            `db:"comments_count"`
		ImagePath      string         `db:"image_path"`
		Tags           pq.StringArray `db:"tags"`
	}
	var rows []row
	err := sqlx.SelectContext(ctx, db, &rows, `
        SELECT
            m.public_id,
            m.short_id,
            m.title,
            u.public_id AS author_public_id,
            u.username  AS author_username,
            m.created_at,
            m.likes_count,
            m.comments_count,
            m.image_path,
            m.tags
        FROM matchups m
        JOIN users u ON u.id = m.author_id
        LEFT JOIN communities c ON c.id = m.community_id
        WHERE m.bracket_id IS NULL
          AND m.visibility = 'public'
          AND (m.community_id IS NULL OR (c.id IS NOT NULL AND c.deleted_at IS NULL))
          AND u.deleted_at IS NULL
          AND (
                LOWER(m.title) LIKE $1
             OR EXISTS (SELECT 1 FROM matchup_details d WHERE d.matchup_id = m.id AND LOWER(d.content) LIKE $1)
             OR EXISTS (SELECT 1 FROM unnest(m.tags) tag WHERE LOWER(tag) LIKE $1)
          )
        ORDER BY m.likes_count DESC, m.created_at DESC
        LIMIT $2`, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search matchups: %w", err)
	}
	out := make([]*searchv1.SearchMatchup, 0, len(rows))
	for _, r := range rows {
		short := ""
		if r.ShortID != nil {
			short = *r.ShortID
		}
		var imgURL *string
		if r.ImagePath != "" {
			u := r.ImagePath
			imgURL = &u
		}
		out = append(out, &searchv1.SearchMatchup{
			Id:             r.PublicID,
			ShortId:        short,
			Title:          r.Title,
			AuthorId:       r.AuthorPublicID,
			AuthorUsername: r.AuthorUsername,
			CreatedAt:      r.CreatedAt.UTC().Format(time.RFC3339),
			LikesCount:     int32(r.LikesCount),
			CommentsCount:  int32(r.CommentsCount),
			ImageUrl:       imgURL,
			Tags:           []string(r.Tags),
		})
	}
	return out, nil
}

// ------------ brackets ------------

func searchBrackets(ctx context.Context, db sqlx.ExtContext, pattern string, limit int, viewerID uint, hasViewer bool) ([]*searchv1.SearchBracket, error) {
	_ = viewerID
	_ = hasViewer
	type row struct {
		PublicID       string         `db:"public_id"`
		ShortID        *string        `db:"short_id"`
		Title          string         `db:"title"`
		Description    string         `db:"description"`
		AuthorPublicID string         `db:"author_public_id"`
		AuthorUsername string         `db:"author_username"`
		CreatedAt      time.Time      `db:"created_at"`
		Size           int            `db:"size"`
		LikesCount     int            `db:"likes_count"`
		Tags           pq.StringArray `db:"tags"`
	}
	var rows []row
	err := sqlx.SelectContext(ctx, db, &rows, `
        SELECT
            b.public_id,
            b.short_id,
            b.title,
            b.description,
            u.public_id AS author_public_id,
            u.username  AS author_username,
            b.created_at,
            b.size,
            b.likes_count,
            b.tags
        FROM brackets b
        JOIN users u ON u.id = b.author_id
        LEFT JOIN communities c ON c.id = b.community_id
        WHERE b.visibility = 'public'
          AND (b.community_id IS NULL OR (c.id IS NOT NULL AND c.deleted_at IS NULL))
          AND u.deleted_at IS NULL
          AND (
                LOWER(b.title) LIKE $1
             OR LOWER(b.description) LIKE $1
             OR EXISTS (SELECT 1 FROM unnest(b.tags) tag WHERE LOWER(tag) LIKE $1)
          )
        ORDER BY b.likes_count DESC, b.created_at DESC
        LIMIT $2`, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search brackets: %w", err)
	}
	out := make([]*searchv1.SearchBracket, 0, len(rows))
	for _, r := range rows {
		short := ""
		if r.ShortID != nil {
			short = *r.ShortID
		}
		out = append(out, &searchv1.SearchBracket{
			Id:             r.PublicID,
			ShortId:        short,
			Title:          r.Title,
			Description:    r.Description,
			AuthorId:       r.AuthorPublicID,
			AuthorUsername: r.AuthorUsername,
			CreatedAt:      r.CreatedAt.UTC().Format(time.RFC3339),
			Size:           int32(r.Size),
			LikesCount:     int32(r.LikesCount),
			Tags:           []string(r.Tags),
		})
	}
	return out, nil
}

// ------------ communities ------------

func searchCommunities(ctx context.Context, db sqlx.ExtContext, pattern string, limit int) ([]*searchv1.SearchCommunity, error) {
	type row struct {
		PublicID    string         `db:"public_id"`
		Slug        string         `db:"slug"`
		Name        string         `db:"name"`
		Description string         `db:"description"`
		AvatarPath  string         `db:"avatar_path"`
		BannerPath  string         `db:"banner_path"`
		MemberCount int64          `db:"member_count"`
		Tags        pq.StringArray `db:"tags"`
	}
	var rows []row
	err := sqlx.SelectContext(ctx, db, &rows, `
        SELECT public_id, slug, name, description, avatar_path, banner_path, member_count, tags
        FROM communities
        WHERE deleted_at IS NULL
          AND privacy = 'public'
          AND (
                LOWER(name) LIKE $1
             OR LOWER(slug) LIKE $1
             OR LOWER(description) LIKE $1
             OR EXISTS (SELECT 1 FROM unnest(tags) tag WHERE LOWER(tag) LIKE $1)
          )
        ORDER BY member_count DESC, id DESC
        LIMIT $2`, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search communities: %w", err)
	}
	out := make([]*searchv1.SearchCommunity, 0, len(rows))
	for _, r := range rows {
		out = append(out, &searchv1.SearchCommunity{
			Id:          r.PublicID,
			Slug:        r.Slug,
			Name:        r.Name,
			Description: r.Description,
			AvatarPath:  r.AvatarPath,
			BannerPath:  r.BannerPath,
			MemberCount: r.MemberCount,
			Tags:        []string(r.Tags),
		})
	}
	return out, nil
}

// ------------ users ------------

func searchUsers(ctx context.Context, db sqlx.ExtContext, pattern string, limit int) ([]*searchv1.SearchUser, error) {
	type row struct {
		PublicID       string  `db:"public_id"`
		Username       string  `db:"username"`
		AvatarPath     string  `db:"avatar_path"`
		FollowersCount int64   `db:"followers_count"`
		Bio            *string `db:"bio"`
	}
	var rows []row
	err := sqlx.SelectContext(ctx, db, &rows, `
        SELECT public_id, username, avatar_path, followers_count, bio
        FROM users
        WHERE deleted_at IS NULL
          AND banned_at IS NULL
          AND (
                LOWER(username) LIKE $1
             OR LOWER(COALESCE(bio, '')) LIKE $1
          )
        ORDER BY followers_count DESC, id DESC
        LIMIT $2`, pattern, limit)
	if err != nil {
		// `bio` column may not exist on older schemas; degrade.
		if strings.Contains(err.Error(), "bio") {
			err2 := sqlx.SelectContext(ctx, db, &rows, `
                SELECT public_id, username, avatar_path, followers_count, NULL::text AS bio
                FROM users
                WHERE deleted_at IS NULL
                  AND banned_at IS NULL
                  AND LOWER(username) LIKE $1
                ORDER BY followers_count DESC, id DESC
                LIMIT $2`, pattern, limit)
			if err2 != nil {
				return nil, fmt.Errorf("search users: %w", err2)
			}
		} else {
			return nil, fmt.Errorf("search users: %w", err)
		}
	}
	out := make([]*searchv1.SearchUser, 0, len(rows))
	for _, r := range rows {
		var bio *string
		if r.Bio != nil && *r.Bio != "" {
			b := *r.Bio
			bio = &b
		}
		out = append(out, &searchv1.SearchUser{
			Id:             r.PublicID,
			Username:       r.Username,
			AvatarPath:     r.AvatarPath,
			FollowersCount: r.FollowersCount,
			Bio:            bio,
		})
	}
	return out, nil
}
