package controllers

import (
	"context"
	"database/sql"
	"errors"

	adminv1 "Matchup/gen/admin/v1"
	"Matchup/gen/admin/v1/adminv1connect"
	userv1 "Matchup/gen/user/v1"
	"Matchup/models"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
)

var _ adminv1connect.AdminServiceHandler = (*AdminHandler)(nil)

// AdminHandler implements adminv1connect.AdminServiceHandler.
type AdminHandler struct {
	DB     *sqlx.DB
	ReadDB *sqlx.DB
}

func (h *AdminHandler) ListUsers(ctx context.Context, req *connect.Request[adminv1.ListUsersRequest]) (*connect.Response[adminv1.ListUsersResponse], error) {
	// Pure-read RPC — safe to serve from the replica.
	db := dbForRead(ctx, h.DB, h.ReadDB)

	page := int(req.Msg.GetPage())
	if page < 1 {
		page = 1
	}
	limit := int(req.Msg.GetLimit())
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	var total int64
	if err := sqlx.GetContext(ctx, db, &total, "SELECT COUNT(*) FROM users"); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var users []models.User
	if err := sqlx.SelectContext(ctx, db, &users,
		"SELECT * FROM users ORDER BY created_at DESC LIMIT $1 OFFSET $2", limit, offset); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	protoUsers := make([]*userv1.UserProfile, len(users))
	for i := range users {
		users[i].ProcessAvatarPath()
		protoUsers[i] = userToProto(&users[i])
	}

	return connect.NewResponse(&adminv1.ListUsersResponse{
		Users:      protoUsers,
		Pagination: paginationToProto(page, limit, total),
	}), nil
}

func (h *AdminHandler) UpdateUserRole(ctx context.Context, req *connect.Request[adminv1.UpdateUserRoleRequest]) (*connect.Response[adminv1.UpdateUserRoleResponse], error) {
	user, err := resolveUserByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	}

	if _, err := h.DB.ExecContext(ctx,
		"UPDATE users SET is_admin = $1 WHERE id = $2", req.Msg.IsAdmin, user.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	user.IsAdmin = req.Msg.IsAdmin
	user.ProcessAvatarPath()

	return connect.NewResponse(&adminv1.UpdateUserRoleResponse{
		User: userToProto(user),
	}), nil
}

func (h *AdminHandler) DeleteUser(ctx context.Context, req *connect.Request[adminv1.DeleteUserRequest]) (*connect.Response[adminv1.DeleteUserResponse], error) {
	user, err := resolveUserByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	}
	// Reuses the shared cascade helper — same code path the 30-day
	// hard-delete cron runs. Keeps admin-immediate + cron-scheduled
	// deletes on identical semantics.
	if err := hardDeleteUser(ctx, h.DB, user.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&adminv1.DeleteUserResponse{Message: "user deleted"}), nil
}

func (h *AdminHandler) ListMatchups(ctx context.Context, req *connect.Request[adminv1.ListMatchupsRequest]) (*connect.Response[adminv1.ListMatchupsResponse], error) {
	// Pure-read RPC — safe to serve from the replica.
	db := dbForRead(ctx, h.DB, h.ReadDB)

	page := int(req.Msg.GetPage())
	if page < 1 {
		page = 1
	}
	limit := int(req.Msg.GetLimit())
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	var total int64
	if err := sqlx.GetContext(ctx, db, &total, "SELECT COUNT(*) FROM matchups WHERE bracket_id IS NULL"); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var matchups []models.Matchup
	if err := sqlx.SelectContext(ctx, db, &matchups,
		"SELECT * FROM matchups WHERE bracket_id IS NULL ORDER BY created_at DESC LIMIT $1 OFFSET $2", limit, offset); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if len(matchups) > 0 {
		matchupIDs := make([]uint, len(matchups))
		authorIDs := make([]uint, len(matchups))
		for i := range matchups {
			matchupIDs[i] = matchups[i].ID
			authorIDs[i] = matchups[i].AuthorID
		}

		itemQuery, itemArgs, _ := sqlx.In("SELECT * FROM matchup_items WHERE matchup_id IN (?)", matchupIDs)
		itemQuery = db.Rebind(itemQuery)
		var allItems []models.MatchupItem
		_ = sqlx.SelectContext(ctx, db, &allItems, itemQuery, itemArgs...)
		// Merge live Redis vote deltas before fanning items into the
		// per-matchup map — copies in the map are detached afterwards.
		applyVoteDeltasToItems(ctx, allItems)
		itemsByMatchup := make(map[uint][]models.MatchupItem)
		for _, item := range allItems {
			itemsByMatchup[item.MatchupID] = append(itemsByMatchup[item.MatchupID], item)
		}

		authorQuery, authorArgs, _ := sqlx.In("SELECT * FROM users WHERE id IN (?)", authorIDs)
		authorQuery = db.Rebind(authorQuery)
		var authors []models.User
		_ = sqlx.SelectContext(ctx, db, &authors, authorQuery, authorArgs...)
		authorMap := make(map[uint]models.User)
		for _, a := range authors {
			a.ProcessAvatarPath()
			authorMap[a.ID] = a
		}

		for i := range matchups {
			matchups[i].Items = itemsByMatchup[matchups[i].ID]
			if a, ok := authorMap[matchups[i].AuthorID]; ok {
				matchups[i].Author = a
			}
		}
	}

	// Admin needs the body — batch-load from matchup_details.
	contentMap, _ := models.LoadMatchupContents(db, func() []uint {
		ids := make([]uint, len(matchups))
		for i := range matchups {
			ids[i] = matchups[i].ID
		}
		return ids
	}())

	// LikesCount lives on each matchup row (migration 006).
	protoMatchups := make([]*adminv1.AdminMatchupData, len(matchups))
	for i := range matchups {
		matchups[i].Content = contentMap[matchups[i].ID]
		protoMatchups[i] = adminMatchupToProto(db, &matchups[i])
	}

	return connect.NewResponse(&adminv1.ListMatchupsResponse{
		Matchups:   protoMatchups,
		Pagination: paginationToProto(page, limit, total),
	}), nil
}

func (h *AdminHandler) DeleteMatchup(ctx context.Context, req *connect.Request[adminv1.DeleteMatchupRequest]) (*connect.Response[adminv1.DeleteMatchupResponse], error) {
	matchupRecord, err := resolveMatchupByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("matchup not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var m models.Matchup
	if err := sqlx.GetContext(ctx, h.DB, &m, "SELECT * FROM matchups WHERE id = $1", matchupRecord.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("matchup not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if err := deleteMatchupCascadeStandalone(h.DB, &m); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&adminv1.DeleteMatchupResponse{Message: "matchup deleted"}), nil
}

func (h *AdminHandler) ListBrackets(ctx context.Context, req *connect.Request[adminv1.ListBracketsRequest]) (*connect.Response[adminv1.ListBracketsResponse], error) {
	// Pure-read RPC — safe to serve from the replica.
	db := dbForRead(ctx, h.DB, h.ReadDB)

	page := int(req.Msg.GetPage())
	if page < 1 {
		page = 1
	}
	limit := int(req.Msg.GetLimit())
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	var total int64
	if err := sqlx.GetContext(ctx, db, &total, "SELECT COUNT(*) FROM brackets"); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var brackets []models.Bracket
	if err := sqlx.SelectContext(ctx, db, &brackets,
		"SELECT * FROM brackets ORDER BY created_at DESC LIMIT $1 OFFSET $2", limit, offset); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if len(brackets) > 0 {
		authorIDs := make([]uint, len(brackets))
		for i, b := range brackets {
			authorIDs[i] = b.AuthorID
		}
		query, inArgs, err := sqlx.In("SELECT * FROM users WHERE id IN (?)", authorIDs)
		if err == nil {
			query = db.Rebind(query)
			var authors []models.User
			if err := sqlx.SelectContext(ctx, db, &authors, query, inArgs...); err == nil {
				authorMap := make(map[uint]models.User, len(authors))
				for _, a := range authors {
					a.ProcessAvatarPath()
					authorMap[a.ID] = a
				}
				for i := range brackets {
					if a, ok := authorMap[brackets[i].AuthorID]; ok {
						brackets[i].Author = a
					}
				}
			}
		}
	}

	// LikesCount lives on each bracket row (migration 006).
	protoBrackets := make([]*adminv1.AdminBracketData, len(brackets))
	for i := range brackets {
		authorUsername := ""
		if brackets[i].Author.ID != 0 {
			authorUsername = brackets[i].Author.Username
		}
		protoBrackets[i] = adminBracketToProto(&brackets[i], authorUsername)
	}

	return connect.NewResponse(&adminv1.ListBracketsResponse{
		Brackets:   protoBrackets,
		Pagination: paginationToProto(page, limit, total),
	}), nil
}

func (h *AdminHandler) DeleteBracket(ctx context.Context, req *connect.Request[adminv1.DeleteBracketRequest]) (*connect.Response[adminv1.DeleteBracketResponse], error) {
	bracketRecord, err := resolveBracketByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var bracket models.Bracket
	if err := sqlx.GetContext(ctx, h.DB, &bracket, "SELECT * FROM brackets WHERE id = $1", bracketRecord.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	s := &Server{DB: h.DB}
	if err := s.deleteBracketCascade(&bracket); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&adminv1.DeleteBracketResponse{Message: "bracket deleted"}), nil
}
