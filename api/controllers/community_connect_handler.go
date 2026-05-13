package controllers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	appdb "Matchup/db"
	communityv1 "Matchup/gen/community/v1"
	"Matchup/gen/community/v1/communityv1connect"
	"Matchup/models"
	"Matchup/utils/slug"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// CommunityHandler implements communityv1connect.CommunityServiceHandler.
// Backed by both a writer DB (for mutating handlers) and a reader DB
// (for read-only paths). dbForRead picks between them based on the
// request context, same pattern as the other handlers.
type CommunityHandler struct {
	DB     *sqlx.DB
	ReadDB *sqlx.DB
}

var _ communityv1connect.CommunityServiceHandler = (*CommunityHandler)(nil)

// Role constants — keep in lockstep with the CHECK constraint in
// migrations/027_communities.sql.
const (
	roleOwner  = "owner"
	roleMod    = "mod"
	roleMember = "member"
	roleBanned = "banned"
)

// Privacy constants — schema allows three, but v1 only creates 'public'.
const (
	privacyPublic     = "public"
	privacyRestricted = "restricted"
	privacyPrivate    = "private"
)

// resolveCommunityByIdentifier accepts either a public_id (UUID) or a
// slug and returns the matching non-deleted community. Mirrors the
// shape of resolveUserByIdentifier so the handler code reads uniformly.
func resolveCommunityByIdentifier(db sqlx.ExtContext, identifier string) (*models.Community, error) {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" {
		return nil, sql.ErrNoRows
	}
	var c models.Community
	if isUUIDLike(trimmed) {
		err := sqlx.GetContext(context.Background(), db, &c,
			"SELECT * FROM communities WHERE public_id = $1 AND deleted_at IS NULL", trimmed)
		if err == nil {
			return &c, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	err := sqlx.GetContext(context.Background(), db, &c,
		"SELECT * FROM communities WHERE slug = $1 AND deleted_at IS NULL", strings.ToLower(trimmed))
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// viewerRole returns the caller's role in this community, or "" when
// the caller is not authenticated or has no membership. Banned users
// get "banned" — handlers that need to gate on "is a real member" must
// check for that explicitly.
func (h *CommunityHandler) viewerRole(ctx context.Context, communityID uint) string {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok || uid == 0 {
		return ""
	}
	var role string
	err := sqlx.GetContext(ctx, dbForRead(ctx, h.DB, h.ReadDB), &role,
		"SELECT role FROM community_memberships WHERE community_id = $1 AND user_id = $2",
		communityID, uid)
	if err != nil {
		return ""
	}
	return role
}

// requireRole returns the user id if the viewer's role is in `allowed`.
// Otherwise returns the appropriate Connect error (Unauthenticated when
// no JWT, PermissionDenied when authed but role doesn't qualify).
func (h *CommunityHandler) requireRole(ctx context.Context, communityID uint, allowed ...string) (uint, error) {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok || uid == 0 {
		return 0, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	role := h.viewerRole(ctx, communityID)
	for _, a := range allowed {
		if role == a {
			return uid, nil
		}
	}
	return 0, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient role"))
}

// roleAtLeast returns true if `have` ranks >= `min` in owner > mod >
// member ordering. Banned is never "at least" anything.
func roleAtLeast(have, min string) bool {
	rank := map[string]int{roleMember: 1, roleMod: 2, roleOwner: 3}
	return rank[have] >= rank[min]
}

// communityToProto serialises a community row for the wire. viewerRole
// is computed against the caller's membership row (empty when anon or
// non-member) so the frontend can render the right UI in one round-trip.
func (h *CommunityHandler) communityToProto(ctx context.Context, c *models.Community) *communityv1.CommunityData {
	var ownerUsername string
	_ = sqlx.GetContext(ctx, dbForRead(ctx, h.DB, h.ReadDB), &ownerUsername,
		"SELECT username FROM users WHERE id = $1", c.OwnerID)
	var ownerPublicID string
	_ = sqlx.GetContext(ctx, dbForRead(ctx, h.DB, h.ReadDB), &ownerPublicID,
		"SELECT public_id FROM users WHERE id = $1", c.OwnerID)

	return &communityv1.CommunityData{
		Id:            c.PublicID,
		Slug:          c.Slug,
		Name:          c.Name,
		Description:   c.Description,
		AvatarPath:    c.AvatarPath,
		BannerPath:    c.BannerPath,
		Tags:          []string(c.Tags),
		Privacy:       c.Privacy,
		OwnerId:       ownerPublicID,
		OwnerUsername: ownerUsername,
		MemberCount:   c.MemberCount,
		MatchupCount:  c.MatchupCount,
		BracketCount:  c.BracketCount,
		CreatedAt:     c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     c.UpdatedAt.UTC().Format(time.RFC3339),
		ViewerRole:    h.viewerRole(ctx, c.ID),
	}
}

// ----------------------------------------------------------------------
// CRUD
// ----------------------------------------------------------------------

func (h *CommunityHandler) CreateCommunity(ctx context.Context, req *connect.Request[communityv1.CreateCommunityRequest]) (*connect.Response[communityv1.CreateCommunityResponse], error) {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok || uid == 0 {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	// Email verification gate — same posture as CreateMatchup /
	// CreateBracket. Unverified accounts can browse but can't spin up
	// a community.
	var verified sql.NullTime
	if err := sqlx.GetContext(ctx, h.DB, &verified,
		"SELECT email_verified_at FROM users WHERE id = $1", uid); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if !verified.Valid {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			errors.New("verify your email before creating a community"))
	}

	s := slug.Normalize(req.Msg.GetSlug())
	if reason := slug.Validate(s); reason != "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("slug %s", reason))
	}

	name := strings.TrimSpace(req.Msg.GetName())
	if len(name) < 2 || len(name) > 64 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			errors.New("name must be between 2 and 64 characters"))
	}

	desc := strings.TrimSpace(req.Msg.GetDescription())
	if len(desc) > 500 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			errors.New("description must be 500 characters or fewer"))
	}
	// When the user provides a description it must be meaningful —
	// at least 20 chars and not just an echo of the community name.
	// Owners that don't have a good blurb yet can leave it blank;
	// the frontend hides the About section entirely in that case.
	if desc != "" {
		if len(desc) < 20 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				errors.New("description should be at least 20 characters — leave blank if you don't have one yet"))
		}
		if strings.EqualFold(desc, name) {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				errors.New("description shouldn't just repeat the community name — try a sentence or two"))
		}
	}

	privacy := req.Msg.GetPrivacy()
	if privacy == "" {
		privacy = privacyPublic
	}
	// v1 only ships public communities. Schema accepts the others; the
	// API will start accepting them once the restricted/private flows
	// land.
	if privacy != privacyPublic {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			errors.New("only public communities are supported in this version"))
	}

	// normalizeTags lowercases + dedupes + caps at 10. We additionally
	// drop any tag that just echoes the community name — those are
	// always redundant with the title and clutter the chip row.
	tags := filterNameEchoTags(normalizeTags(req.Msg.GetTags()), name)

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer tx.Rollback()

	publicID := appdb.GeneratePublicID()
	var community models.Community
	err = tx.QueryRowxContext(ctx, `
        INSERT INTO communities (public_id, slug, name, description, avatar_path, banner_path, tags, privacy, owner_id, member_count)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 1)
        RETURNING *`,
		publicID, s, name, desc,
		strings.TrimSpace(req.Msg.GetAvatarPath()),
		strings.TrimSpace(req.Msg.GetBannerPath()),
		pq.Array(tags), privacy, uid,
	).StructScan(&community)
	if err != nil {
		// Unique constraint on slug — friendlier error.
		if pgErr, ok := err.(*pq.Error); ok && pgErr.Code == "23505" {
			return nil, connect.NewError(connect.CodeAlreadyExists,
				errors.New("slug is already taken"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Insert the owner membership row inline so the creator is a
	// proper member from t=0. member_count was initialised to 1 above
	// to match.
	_, err = tx.ExecContext(ctx, `
        INSERT INTO community_memberships (community_id, user_id, role)
        VALUES ($1, $2, $3)`,
		community.ID, uid, roleOwner,
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&communityv1.CreateCommunityResponse{
		Community: h.communityToProto(ctx, &community),
	}), nil
}

func (h *CommunityHandler) GetCommunity(ctx context.Context, req *connect.Request[communityv1.GetCommunityRequest]) (*connect.Response[communityv1.GetCommunityResponse], error) {
	c, err := resolveCommunityByIdentifier(dbForRead(ctx, h.DB, h.ReadDB), req.Msg.GetId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("community not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&communityv1.GetCommunityResponse{
		Community: h.communityToProto(ctx, c),
	}), nil
}

func (h *CommunityHandler) GetCommunityBySlug(ctx context.Context, req *connect.Request[communityv1.GetCommunityBySlugRequest]) (*connect.Response[communityv1.GetCommunityBySlugResponse], error) {
	s := slug.Normalize(req.Msg.GetSlug())
	c, err := resolveCommunityByIdentifier(dbForRead(ctx, h.DB, h.ReadDB), s)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("community not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&communityv1.GetCommunityBySlugResponse{
		Community: h.communityToProto(ctx, c),
	}), nil
}

func (h *CommunityHandler) UpdateCommunity(ctx context.Context, req *connect.Request[communityv1.UpdateCommunityRequest]) (*connect.Response[communityv1.UpdateCommunityResponse], error) {
	c, err := resolveCommunityByIdentifier(h.DB, req.Msg.GetId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("community not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if _, err := h.requireRole(ctx, c.ID, roleOwner); err != nil {
		return nil, err
	}

	setClauses := []string{}
	args := []interface{}{}
	add := func(col string, val interface{}) {
		args = append(args, val)
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, len(args)))
	}

	if req.Msg.Name != nil {
		name := strings.TrimSpace(*req.Msg.Name)
		if len(name) < 2 || len(name) > 64 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				errors.New("name must be between 2 and 64 characters"))
		}
		add("name", name)
	}
	if req.Msg.Description != nil {
		desc := strings.TrimSpace(*req.Msg.Description)
		if len(desc) > 500 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				errors.New("description must be 500 characters or fewer"))
		}
		// Same length + name-echo rules as Create. Empty stays valid
		// (clears the description).
		if desc != "" {
			// Resolve the effective name — either the updated value
			// in this request, or the existing community name.
			effectiveName := c.Name
			if req.Msg.Name != nil {
				if n := strings.TrimSpace(*req.Msg.Name); n != "" {
					effectiveName = n
				}
			}
			if len(desc) < 20 {
				return nil, connect.NewError(connect.CodeInvalidArgument,
					errors.New("description should be at least 20 characters — leave blank if you don't have one yet"))
			}
			if strings.EqualFold(desc, effectiveName) {
				return nil, connect.NewError(connect.CodeInvalidArgument,
					errors.New("description shouldn't just repeat the community name — try a sentence or two"))
			}
		}
		add("description", desc)
	}
	if req.Msg.AvatarPath != nil {
		add("avatar_path", strings.TrimSpace(*req.Msg.AvatarPath))
	}
	if req.Msg.BannerPath != nil {
		add("banner_path", strings.TrimSpace(*req.Msg.BannerPath))
	}
	if len(req.Msg.GetTags()) > 0 {
		// Use the effective name (request override OR current) for the
		// name-echo filter so a rename + new tag list both line up.
		effectiveName := c.Name
		if req.Msg.Name != nil {
			if n := strings.TrimSpace(*req.Msg.Name); n != "" {
				effectiveName = n
			}
		}
		add("tags", pq.Array(filterNameEchoTags(normalizeTags(req.Msg.GetTags()), effectiveName)))
	}
	if req.Msg.Privacy != nil {
		p := *req.Msg.Privacy
		if p != privacyPublic {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				errors.New("only public communities are supported in this version"))
		}
		add("privacy", p)
	}
	if len(setClauses) == 0 {
		// No-op update — just return the current state.
		return connect.NewResponse(&communityv1.UpdateCommunityResponse{
			Community: h.communityToProto(ctx, c),
		}), nil
	}
	setClauses = append(setClauses, fmt.Sprintf("updated_at = NOW()"))
	args = append(args, c.ID)
	query := fmt.Sprintf("UPDATE communities SET %s WHERE id = $%d RETURNING *",
		strings.Join(setClauses, ", "), len(args))

	var updated models.Community
	if err := h.DB.QueryRowxContext(ctx, query, args...).StructScan(&updated); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&communityv1.UpdateCommunityResponse{
		Community: h.communityToProto(ctx, &updated),
	}), nil
}

func (h *CommunityHandler) DeleteCommunity(ctx context.Context, req *connect.Request[communityv1.DeleteCommunityRequest]) (*connect.Response[communityv1.DeleteCommunityResponse], error) {
	c, err := resolveCommunityByIdentifier(h.DB, req.Msg.GetId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("community not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if _, err := h.requireRole(ctx, c.ID, roleOwner); err != nil {
		return nil, err
	}
	// Soft delete — recoverable for 30 days via the same cron path
	// that handles user soft-deletes. The matchups + brackets remain
	// attached via community_id but the community itself is hidden.
	if _, err := h.DB.ExecContext(ctx,
		"UPDATE communities SET deleted_at = NOW() WHERE id = $1", c.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&communityv1.DeleteCommunityResponse{Message: "community deleted"}), nil
}

func (h *CommunityHandler) ListCommunities(ctx context.Context, req *connect.Request[communityv1.ListCommunitiesRequest]) (*connect.Response[communityv1.ListCommunitiesResponse], error) {
	limit := int(req.Msg.GetLimit())
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	args := []interface{}{}
	where := []string{"deleted_at IS NULL"}
	if tag := strings.TrimSpace(req.Msg.GetTag()); tag != "" {
		args = append(args, tag)
		where = append(where, fmt.Sprintf("$%d = ANY(tags)", len(args)))
	}
	if q := strings.TrimSpace(req.Msg.GetQuery()); q != "" {
		// Simple ILIKE on name + description. Good enough for v1; real
		// fulltext can come later if directory-search becomes a hot path.
		args = append(args, "%"+q+"%")
		where = append(where, fmt.Sprintf("(name ILIKE $%d OR description ILIKE $%d)", len(args), len(args)))
	}
	if cursor := strings.TrimSpace(req.Msg.GetCursor()); cursor != "" {
		if cursorID, err := strconv.ParseInt(cursor, 10, 64); err == nil {
			args = append(args, cursorID)
			where = append(where, fmt.Sprintf("id < $%d", len(args)))
		}
	}
	args = append(args, limit)
	query := fmt.Sprintf(`
        SELECT * FROM communities
        WHERE %s
        ORDER BY member_count DESC, id DESC
        LIMIT $%d`,
		strings.Join(where, " AND "), len(args))

	var rows []models.Community
	if err := sqlx.SelectContext(ctx, dbForRead(ctx, h.DB, h.ReadDB), &rows, query, args...); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	resp := &communityv1.ListCommunitiesResponse{}
	for i := range rows {
		resp.Communities = append(resp.Communities, h.communityToProto(ctx, &rows[i]))
	}
	if len(rows) == limit {
		resp.NextCursor = strconv.FormatUint(uint64(rows[len(rows)-1].ID), 10)
	}
	return connect.NewResponse(resp), nil
}

// ListMyCommunities — communities the authenticated caller is a
// non-banned member of, ordered by joined_at DESC so the most-
// recently-joined sit at the top of the sidebar. Anonymous callers
// get an empty array (not an error) so the frontend can render the
// section unconditionally without branching on auth state.
func (h *CommunityHandler) ListMyCommunities(ctx context.Context, req *connect.Request[communityv1.ListMyCommunitiesRequest]) (*connect.Response[communityv1.ListMyCommunitiesResponse], error) {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok || uid == 0 {
		return connect.NewResponse(&communityv1.ListMyCommunitiesResponse{}), nil
	}
	limit := int(req.Msg.GetLimit())
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	// One query: join memberships to communities so we get both the
	// community row and the caller's role without a fan-out. Banned
	// memberships are excluded — a banned user shouldn't see the
	// community in their own sidebar.
	type row struct {
		models.Community
		ViewerRole string `db:"viewer_role"`
	}
	var rows []row
	err := sqlx.SelectContext(ctx, dbForRead(ctx, h.DB, h.ReadDB), &rows, `
        SELECT c.*, m.role AS viewer_role
        FROM community_memberships m
        JOIN communities c ON c.id = m.community_id
        WHERE m.user_id = $1
          AND m.role <> 'banned'
          AND c.deleted_at IS NULL
        ORDER BY m.joined_at DESC
        LIMIT $2`, uid, limit)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	resp := &communityv1.ListMyCommunitiesResponse{}
	for i := range rows {
		// Avoid the extra viewer_role lookup inside communityToProto
		// (which would issue one query per community for the same
		// data we already have in `rows[i].ViewerRole`). Build the
		// proto inline; everything else matches communityToProto.
		c := &rows[i].Community
		var ownerUsername string
		_ = sqlx.GetContext(ctx, dbForRead(ctx, h.DB, h.ReadDB), &ownerUsername,
			"SELECT username FROM users WHERE id = $1", c.OwnerID)
		var ownerPublicID string
		_ = sqlx.GetContext(ctx, dbForRead(ctx, h.DB, h.ReadDB), &ownerPublicID,
			"SELECT public_id FROM users WHERE id = $1", c.OwnerID)
		resp.Communities = append(resp.Communities, &communityv1.CommunityData{
			Id:            c.PublicID,
			Slug:          c.Slug,
			Name:          c.Name,
			Description:   c.Description,
			AvatarPath:    c.AvatarPath,
			BannerPath:    c.BannerPath,
			Tags:          []string(c.Tags),
			Privacy:       c.Privacy,
			OwnerId:       ownerPublicID,
			OwnerUsername: ownerUsername,
			MemberCount:   c.MemberCount,
			MatchupCount:  c.MatchupCount,
			BracketCount:  c.BracketCount,
			CreatedAt:     c.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:     c.UpdatedAt.UTC().Format(time.RFC3339),
			ViewerRole:    rows[i].ViewerRole,
		})
	}
	return connect.NewResponse(resp), nil
}

func (h *CommunityHandler) CheckSlugAvailable(ctx context.Context, req *connect.Request[communityv1.CheckSlugAvailableRequest]) (*connect.Response[communityv1.CheckSlugAvailableResponse], error) {
	s := slug.Normalize(req.Msg.GetSlug())
	if reason := slug.Validate(s); reason != "" {
		return connect.NewResponse(&communityv1.CheckSlugAvailableResponse{
			Available: false,
			Reason:    reason,
		}), nil
	}
	var exists bool
	if err := sqlx.GetContext(ctx, dbForRead(ctx, h.DB, h.ReadDB), &exists,
		"SELECT EXISTS(SELECT 1 FROM communities WHERE slug = $1)", s); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	resp := &communityv1.CheckSlugAvailableResponse{Available: !exists}
	if exists {
		resp.Reason = "taken"
		// Best-effort numeric suggestions; if any are also taken the
		// frontend can re-check.
		resp.Suggestions = []string{s + "-2", s + "-3", s + "-hub"}
	}
	return connect.NewResponse(resp), nil
}

// ----------------------------------------------------------------------
// Membership / roles
// ----------------------------------------------------------------------

func (h *CommunityHandler) JoinCommunity(ctx context.Context, req *connect.Request[communityv1.JoinCommunityRequest]) (*connect.Response[communityv1.JoinCommunityResponse], error) {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok || uid == 0 {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	c, err := resolveCommunityByIdentifier(h.DB, req.Msg.GetId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("community not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Check existing membership state. Banned can't re-join.
	var existing struct {
		Role string `db:"role"`
	}
	err = sqlx.GetContext(ctx, h.DB, &existing,
		"SELECT role FROM community_memberships WHERE community_id = $1 AND user_id = $2",
		c.ID, uid)
	if err == nil {
		if existing.Role == roleBanned {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("you are banned from this community"))
		}
		// Already a member — idempotent success.
		return connect.NewResponse(&communityv1.JoinCommunityResponse{
			Community: h.communityToProto(ctx, c),
			Role:      existing.Role,
		}), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// v1: only public communities exist, so any authed user can join.
	// When restricted/private modes light up, gate here on privacy.
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO community_memberships (community_id, user_id, role) VALUES ($1, $2, $3)",
		c.ID, uid, roleMember); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE communities SET member_count = member_count + 1 WHERE id = $1", c.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Re-read so member_count reflects the join in the response.
	updated, _ := resolveCommunityByIdentifier(dbForRead(ctx, h.DB, h.ReadDB), req.Msg.GetId())
	if updated == nil {
		updated = c
	}
	return connect.NewResponse(&communityv1.JoinCommunityResponse{
		Community: h.communityToProto(ctx, updated),
		Role:      roleMember,
	}), nil
}

func (h *CommunityHandler) LeaveCommunity(ctx context.Context, req *connect.Request[communityv1.LeaveCommunityRequest]) (*connect.Response[communityv1.LeaveCommunityResponse], error) {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok || uid == 0 {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	c, err := resolveCommunityByIdentifier(h.DB, req.Msg.GetId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("community not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// Owners cannot leave without transferring ownership first.
	// Transfer flow is not implemented in v1; for now they must delete
	// the community instead.
	if c.OwnerID == uid {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			errors.New("transfer ownership or delete the community before leaving"))
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		"DELETE FROM community_memberships WHERE community_id = $1 AND user_id = $2 AND role <> 'banned'",
		c.ID, uid)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		// Not a member, or banned — silently succeed in the not-a-member
		// case; banned users can't leave (and shouldn't be told the
		// distinction matters).
		return connect.NewResponse(&communityv1.LeaveCommunityResponse{Message: "not a member"}), nil
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE communities SET member_count = GREATEST(member_count - 1, 0) WHERE id = $1", c.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&communityv1.LeaveCommunityResponse{Message: "left community"}), nil
}

func (h *CommunityHandler) ListMembers(ctx context.Context, req *connect.Request[communityv1.ListMembersRequest]) (*connect.Response[communityv1.ListMembersResponse], error) {
	c, err := resolveCommunityByIdentifier(dbForRead(ctx, h.DB, h.ReadDB), req.Msg.GetCommunityId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("community not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	limit := int(req.Msg.GetLimit())
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	args := []interface{}{c.ID}
	where := []string{"m.community_id = $1"}
	if role := strings.TrimSpace(req.Msg.GetRole()); role != "" {
		args = append(args, role)
		where = append(where, fmt.Sprintf("m.role = $%d", len(args)))
	} else {
		where = append(where, "m.role <> 'banned'")
	}
	if cursor := strings.TrimSpace(req.Msg.GetCursor()); cursor != "" {
		args = append(args, cursor)
		where = append(where, fmt.Sprintf("u.public_id < $%d", len(args)))
	}
	args = append(args, limit)
	query := fmt.Sprintf(`
        SELECT u.public_id, u.username, u.avatar_path, m.role, m.joined_at
        FROM community_memberships m
        JOIN users u ON u.id = m.user_id
        WHERE %s
        ORDER BY
            CASE m.role WHEN 'owner' THEN 0 WHEN 'mod' THEN 1 ELSE 2 END,
            u.public_id DESC
        LIMIT $%d`,
		strings.Join(where, " AND "), len(args))

	type row struct {
		PublicID   string    `db:"public_id"`
		Username   string    `db:"username"`
		AvatarPath string    `db:"avatar_path"`
		Role       string    `db:"role"`
		JoinedAt   time.Time `db:"joined_at"`
	}
	var rows []row
	if err := sqlx.SelectContext(ctx, dbForRead(ctx, h.DB, h.ReadDB), &rows, query, args...); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	resp := &communityv1.ListMembersResponse{}
	for _, r := range rows {
		resp.Members = append(resp.Members, &communityv1.CommunityMember{
			Id:         r.PublicID,
			Username:   r.Username,
			AvatarPath: r.AvatarPath,
			Role:       r.Role,
			JoinedAt:   r.JoinedAt.UTC().Format(time.RFC3339),
		})
	}
	if len(rows) == limit {
		resp.NextCursor = rows[len(rows)-1].PublicID
	}
	return connect.NewResponse(resp), nil
}

func (h *CommunityHandler) GetMyMembership(ctx context.Context, req *connect.Request[communityv1.GetMyMembershipRequest]) (*connect.Response[communityv1.GetMyMembershipResponse], error) {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok || uid == 0 {
		return connect.NewResponse(&communityv1.GetMyMembershipResponse{}), nil
	}
	c, err := resolveCommunityByIdentifier(dbForRead(ctx, h.DB, h.ReadDB), req.Msg.GetCommunityId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("community not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	var m models.CommunityMembership
	err = sqlx.GetContext(ctx, dbForRead(ctx, h.DB, h.ReadDB), &m,
		"SELECT * FROM community_memberships WHERE community_id = $1 AND user_id = $2",
		c.ID, uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connect.NewResponse(&communityv1.GetMyMembershipResponse{}), nil
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&communityv1.GetMyMembershipResponse{
		Role:     m.Role,
		JoinedAt: m.JoinedAt.UTC().Format(time.RFC3339),
	}), nil
}

func (h *CommunityHandler) UpdateMemberRole(ctx context.Context, req *connect.Request[communityv1.UpdateMemberRoleRequest]) (*connect.Response[communityv1.UpdateMemberRoleResponse], error) {
	c, err := resolveCommunityByIdentifier(h.DB, req.Msg.GetCommunityId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("community not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if _, err := h.requireRole(ctx, c.ID, roleOwner); err != nil {
		return nil, err
	}
	target, err := resolveUserByIdentifier(h.DB, req.Msg.GetUserId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	}
	if target.ID == c.OwnerID {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			errors.New("cannot change the owner's role"))
	}
	newRole := req.Msg.GetRole()
	if newRole != roleMod && newRole != roleMember {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			errors.New("role must be 'mod' or 'member'"))
	}

	res, err := h.DB.ExecContext(ctx,
		"UPDATE community_memberships SET role = $1 WHERE community_id = $2 AND user_id = $3 AND role NOT IN ('owner', 'banned')",
		newRole, c.ID, target.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return nil, connect.NewError(connect.CodeNotFound,
			errors.New("user is not a member of this community"))
	}
	var joinedAt time.Time
	_ = sqlx.GetContext(ctx, h.DB, &joinedAt,
		"SELECT joined_at FROM community_memberships WHERE community_id = $1 AND user_id = $2",
		c.ID, target.ID)
	return connect.NewResponse(&communityv1.UpdateMemberRoleResponse{
		Member: &communityv1.CommunityMember{
			Id:         target.PublicID,
			Username:   target.Username,
			AvatarPath: target.AvatarPath,
			Role:       newRole,
			JoinedAt:   joinedAt.UTC().Format(time.RFC3339),
		},
	}), nil
}

func (h *CommunityHandler) RemoveMember(ctx context.Context, req *connect.Request[communityv1.RemoveMemberRequest]) (*connect.Response[communityv1.RemoveMemberResponse], error) {
	c, err := resolveCommunityByIdentifier(h.DB, req.Msg.GetCommunityId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("community not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if _, err := h.requireRole(ctx, c.ID, roleOwner, roleMod); err != nil {
		return nil, err
	}
	target, err := resolveUserByIdentifier(h.DB, req.Msg.GetUserId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	}
	if target.ID == c.OwnerID {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			errors.New("cannot remove the owner"))
	}
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		"DELETE FROM community_memberships WHERE community_id = $1 AND user_id = $2 AND role NOT IN ('owner', 'banned')",
		c.ID, target.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if affected, _ := res.RowsAffected(); affected > 0 {
		if _, err := tx.ExecContext(ctx,
			"UPDATE communities SET member_count = GREATEST(member_count - 1, 0) WHERE id = $1", c.ID); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&communityv1.RemoveMemberResponse{Message: "member removed"}), nil
}

func (h *CommunityHandler) BanMember(ctx context.Context, req *connect.Request[communityv1.BanMemberRequest]) (*connect.Response[communityv1.BanMemberResponse], error) {
	c, err := resolveCommunityByIdentifier(h.DB, req.Msg.GetCommunityId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("community not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	actorID, err := h.requireRole(ctx, c.ID, roleOwner, roleMod)
	if err != nil {
		return nil, err
	}
	target, err := resolveUserByIdentifier(h.DB, req.Msg.GetUserId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	}
	if target.ID == c.OwnerID {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			errors.New("cannot ban the owner"))
	}

	reason := strings.TrimSpace(req.Msg.GetReason())
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer tx.Rollback()
	// Upsert: if the target was a member they get flipped to banned;
	// if they weren't a member, insert a banned row so a future
	// JoinCommunity is rejected.
	res, err := tx.ExecContext(ctx, `
        INSERT INTO community_memberships (community_id, user_id, role, banned_at, banned_by_user_id, banned_reason)
        VALUES ($1, $2, 'banned', NOW(), $3, NULLIF($4, ''))
        ON CONFLICT (community_id, user_id)
        DO UPDATE SET role = 'banned', banned_at = NOW(), banned_by_user_id = $3, banned_reason = NULLIF($4, '')
        WHERE community_memberships.role <> 'owner'`,
		c.ID, target.ID, actorID, reason)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if affected, _ := res.RowsAffected(); affected > 0 {
		// If we banned an active member, decrement count.
		if _, err := tx.ExecContext(ctx,
			"UPDATE communities SET member_count = GREATEST(member_count - 1, 0) WHERE id = $1", c.ID); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&communityv1.BanMemberResponse{Message: "member banned"}), nil
}

func (h *CommunityHandler) UnbanMember(ctx context.Context, req *connect.Request[communityv1.UnbanMemberRequest]) (*connect.Response[communityv1.UnbanMemberResponse], error) {
	c, err := resolveCommunityByIdentifier(h.DB, req.Msg.GetCommunityId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("community not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if _, err := h.requireRole(ctx, c.ID, roleOwner, roleMod); err != nil {
		return nil, err
	}
	target, err := resolveUserByIdentifier(h.DB, req.Msg.GetUserId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	}
	if _, err := h.DB.ExecContext(ctx,
		"DELETE FROM community_memberships WHERE community_id = $1 AND user_id = $2 AND role = 'banned'",
		c.ID, target.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&communityv1.UnbanMemberResponse{Message: "ban lifted"}), nil
}

// ----------------------------------------------------------------------
// Rules
// ----------------------------------------------------------------------

func (h *CommunityHandler) ListRules(ctx context.Context, req *connect.Request[communityv1.ListRulesRequest]) (*connect.Response[communityv1.ListRulesResponse], error) {
	c, err := resolveCommunityByIdentifier(dbForRead(ctx, h.DB, h.ReadDB), req.Msg.GetCommunityId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("community not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	var rows []models.CommunityRule
	if err := sqlx.SelectContext(ctx, dbForRead(ctx, h.DB, h.ReadDB), &rows,
		"SELECT * FROM community_rules WHERE community_id = $1 ORDER BY position ASC, id ASC", c.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	resp := &communityv1.ListRulesResponse{}
	for _, r := range rows {
		resp.Rules = append(resp.Rules, &communityv1.CommunityRule{
			Id:       int64(r.ID),
			Position: r.Position,
			Title:    r.Title,
			Body:     r.Body,
		})
	}
	return connect.NewResponse(resp), nil
}

func (h *CommunityHandler) SetRules(ctx context.Context, req *connect.Request[communityv1.SetRulesRequest]) (*connect.Response[communityv1.SetRulesResponse], error) {
	c, err := resolveCommunityByIdentifier(h.DB, req.Msg.GetCommunityId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("community not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if _, err := h.requireRole(ctx, c.ID, roleOwner); err != nil {
		return nil, err
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM community_rules WHERE community_id = $1", c.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	for i, r := range req.Msg.GetRules() {
		title := strings.TrimSpace(r.GetTitle())
		if title == "" {
			continue
		}
		body := strings.TrimSpace(r.GetBody())
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO community_rules (community_id, position, title, body) VALUES ($1, $2, $3, $4)",
			c.ID, int32(i), title, body); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// Re-read to return the canonical post-write state.
	listReq := connect.NewRequest(&communityv1.ListRulesRequest{CommunityId: req.Msg.GetCommunityId()})
	listResp, err := h.ListRules(ctx, listReq)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&communityv1.SetRulesResponse{Rules: listResp.Msg.GetRules()}), nil
}

// ----------------------------------------------------------------------
// Community feed
// ----------------------------------------------------------------------

// GetCommunityFeed returns the matchups + brackets scoped to this
// community, sorted by created_at DESC. v1 fetches up to `limit` of
// each in parallel; the frontend interleaves them by created_at on
// the client. Cursor pagination is RFC3339 — pass the older of the
// last seen matchup/bracket created_at to get the next page.
func (h *CommunityHandler) GetCommunityFeed(ctx context.Context, req *connect.Request[communityv1.GetCommunityFeedRequest]) (*connect.Response[communityv1.GetCommunityFeedResponse], error) {
	c, err := resolveCommunityByIdentifier(dbForRead(ctx, h.DB, h.ReadDB), req.Msg.GetCommunityId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("community not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	limit := int(req.Msg.GetLimit())
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var before time.Time
	if cursor := strings.TrimSpace(req.Msg.GetCursor()); cursor != "" {
		t, perr := time.Parse(time.RFC3339, cursor)
		if perr != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cursor: %w", perr))
		}
		before = t
	}

	db := dbForRead(ctx, h.DB, h.ReadDB)

	// Matchups for this community. Standalone bracket matchups (the
	// child rows of a bracket) are filtered out — community pages
	// show top-level user-created matchups, not the auto-generated
	// per-round matchups inside a bracket.
	matchupQuery := `
        SELECT * FROM matchups
        WHERE community_id = $1 AND bracket_id IS NULL`
	matchupArgs := []interface{}{c.ID}
	if !before.IsZero() {
		matchupArgs = append(matchupArgs, before)
		matchupQuery += fmt.Sprintf(" AND created_at < $%d", len(matchupArgs))
	}
	matchupArgs = append(matchupArgs, limit)
	matchupQuery += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", len(matchupArgs))

	var matchups []models.Matchup
	if err := sqlx.SelectContext(ctx, db, &matchups, matchupQuery, matchupArgs...); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Brackets for this community.
	bracketQuery := `SELECT * FROM brackets WHERE community_id = $1`
	bracketArgs := []interface{}{c.ID}
	if !before.IsZero() {
		bracketArgs = append(bracketArgs, before)
		bracketQuery += fmt.Sprintf(" AND created_at < $%d", len(bracketArgs))
	}
	bracketArgs = append(bracketArgs, limit)
	bracketQuery += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", len(bracketArgs))

	var brackets []models.Bracket
	if err := sqlx.SelectContext(ctx, db, &brackets, bracketQuery, bracketArgs...); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Hydrate author rows and items for matchups (matchupToProto
	// expects them populated). Best-effort — a failure here just
	// leaves a card with a missing field, not a 500.
	for i := range matchups {
		_ = sqlx.GetContext(ctx, db, &matchups[i].Author,
			"SELECT * FROM users WHERE id = $1", matchups[i].AuthorID)
		_ = sqlx.SelectContext(ctx, db, &matchups[i].Items,
			"SELECT * FROM matchup_items WHERE matchup_id = $1 ORDER BY id ASC", matchups[i].ID)
	}
	for i := range brackets {
		_ = sqlx.GetContext(ctx, db, &brackets[i].Author,
			"SELECT * FROM users WHERE id = $1", brackets[i].AuthorID)
	}

	resp := &communityv1.GetCommunityFeedResponse{}
	for i := range matchups {
		resp.Matchups = append(resp.Matchups, matchupToProto(db, &matchups[i], nil))
	}
	for i := range brackets {
		resp.Brackets = append(resp.Brackets, bracketToProto(db, &brackets[i]))
	}

	// Cursor = oldest created_at across both lists. Frontend uses
	// this on next-page fetch so neither list misses items.
	var oldest time.Time
	if len(matchups) > 0 {
		oldest = matchups[len(matchups)-1].CreatedAt
	}
	if len(brackets) > 0 {
		bOldest := brackets[len(brackets)-1].CreatedAt
		if oldest.IsZero() || bOldest.Before(oldest) {
			oldest = bOldest
		}
	}
	// Only emit a cursor if we hit the page limit on at least one
	// list — otherwise there's nothing more to fetch.
	if len(matchups) == limit || len(brackets) == limit {
		if !oldest.IsZero() {
			resp.NextCursor = oldest.UTC().Format(time.RFC3339)
		}
	}
	return connect.NewResponse(resp), nil
}

// ----------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------

// filterNameEchoTags drops any tag that case-insensitively matches
// the community name. Those tags duplicate info already in the title
// and clutter the chip row. Defensive both on Create and Update so a
// rename can't leave a stale name-echo tag behind.
func filterNameEchoTags(tags []string, name string) []string {
	if name == "" {
		return tags
	}
	normalized := strings.ToLower(strings.TrimSpace(name))
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		if strings.ToLower(strings.TrimSpace(t)) == normalized {
			continue
		}
		out = append(out, t)
	}
	return out
}

// normalizeTags lowercases, trims, dedupes, and caps the tag list at 10.
func normalizeTags(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, t := range in {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		// Allow letters, digits, and hyphens — same shape as a slug.
		safe := true
		for _, r := range t {
			if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
				safe = false
				break
			}
		}
		if !safe || len(t) > 30 {
			continue
		}
		seen[t] = true
		out = append(out, t)
		if len(out) >= 10 {
			break
		}
	}
	return out
}
