package controllers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	appdb "Matchup/db"
	likev1 "Matchup/gen/like/v1"
	"Matchup/gen/like/v1/likev1connect"
	"Matchup/models"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
)

// LikeHandler implements likev1connect.LikeServiceHandler.
type LikeHandler struct {
	DB     *sqlx.DB
	ReadDB *sqlx.DB
}

var _ likev1connect.LikeServiceHandler = (*LikeHandler)(nil)

func (h *LikeHandler) GetLikes(ctx context.Context, req *connect.Request[likev1.GetLikesRequest]) (*connect.Response[likev1.GetLikesResponse], error) {
	// Pure-read RPC — safe to serve from the replica.
	db := dbForRead(ctx, h.DB, h.ReadDB)

	matchup, err := resolveMatchupByIdentifier(db, req.Msg.MatchupId)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid matchup ID"))
		}
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("matchup not found"))
	}

	var m models.Matchup
	if err := sqlx.GetContext(ctx, db, &m, "SELECT * FROM matchups WHERE id = $1", matchup.ID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("matchup not found"))
	}
	if err := sqlx.GetContext(ctx, db, &m.Author, "SELECT * FROM users WHERE id = $1", m.AuthorID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("matchup not found"))
	}
	m.Author.ProcessAvatarPath()

	viewerID, hasViewer := optionalViewerFromCtx(ctx)
	allowed, reason, err := canViewUserContent(db, viewerID, hasViewer, &m.Author, m.Visibility, httpctx.IsAdminRequest(ctx))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("%s", visibilityErrorMessage(reason)))
	}

	var likes []models.Like
	like := models.Like{}
	ls, err := like.GetLikesInfo(db, matchup.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	likes = *ls

	userIDs := make([]uint, 0, len(likes))
	matchupIDs := make([]uint, 0, len(likes))
	for _, l := range likes {
		userIDs = append(userIDs, l.UserID)
		matchupIDs = append(matchupIDs, l.MatchupID)
	}
	userPublicIDs := loadUserPublicIDMap(db, userIDs)
	matchupPublicIDs := loadMatchupPublicIDMap(db, matchupIDs)

	protos := make([]*likev1.LikeData, len(likes))
	for i, l := range likes {
		protos[i] = likeToProto(l, userPublicIDs[l.UserID], matchupPublicIDs[l.MatchupID])
	}
	return connect.NewResponse(&likev1.GetLikesResponse{Likes: protos}), nil
}

func (h *LikeHandler) LikeMatchup(ctx context.Context, req *connect.Request[likev1.LikeMatchupRequest]) (*connect.Response[likev1.LikeMatchupResponse], error) {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}

	matchupRecord, err := resolveMatchupByIdentifier(h.DB, req.Msg.MatchupId)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid matchup ID"))
		}
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("matchup not found"))
	}

	var m models.Matchup
	if err := sqlx.GetContext(ctx, h.DB, &m, "SELECT * FROM matchups WHERE id = $1", matchupRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("matchup not found"))
	}
	if err := sqlx.GetContext(ctx, h.DB, &m.Author, "SELECT * FROM users WHERE id = $1", m.AuthorID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("matchup not found"))
	}
	m.Author.ProcessAvatarPath()

	allowed, reason, err := canViewUserContent(h.DB, uid, true, &m.Author, m.Visibility, httpctx.IsAdminRequest(ctx))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("%s", visibilityErrorMessage(reason)))
	}

	isOwner := uid == m.AuthorID
	if !isOwner {
		if m.Status != matchupStatusCompleted && !isMatchupOpenStatus(m.Status) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("matchup is not active"))
		}
		if m.Status != matchupStatusCompleted && m.EndTime != nil && time.Now().After(*m.EndTime) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("matchup has ended"))
		}
		if m.BracketID != nil && m.Status != matchupStatusCompleted {
			var bracket models.Bracket
			if err := sqlx.GetContext(ctx, h.DB, &bracket, "SELECT * FROM brackets WHERE id = $1", *m.BracketID); err != nil {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("bracket not found"))
			}
			if bracket.Status != "active" {
				return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("bracket is not active"))
			}
			if m.Round == nil || *m.Round != bracket.CurrentRound {
				return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("matchup is not in the active round"))
			}
		}
	}

	var existing models.Like
	err = sqlx.GetContext(ctx, h.DB, &existing, "SELECT * FROM likes WHERE user_id = $1 AND matchup_id = $2", uid, matchupRecord.ID)
	if err == nil {
		return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("already liked this matchup"))
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	like := models.Like{
		PublicID:  appdb.GeneratePublicID(),
		UserID:    uid,
		MatchupID: matchupRecord.ID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if _, err := h.DB.ExecContext(ctx,
		"INSERT INTO likes (public_id, user_id, matchup_id, created_at, updated_at) VALUES ($1, $2, $3, $4, $5)",
		like.PublicID, like.UserID, like.MatchupID, like.CreatedAt, like.UpdatedAt); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if m.BracketID != nil {
		invalidateBracketSummaryCache(*m.BracketID)
	}
	invalidateHomeSummaryCache(m.AuthorID)

	userPublicID := resolveUserPublicID(h.DB, nil, uid)
	matchupPublicID := matchupRecord.PublicID
	resp := connect.NewResponse(&likev1.LikeMatchupResponse{Like: likeToProto(like, userPublicID, matchupPublicID)})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

func (h *LikeHandler) UnlikeMatchup(ctx context.Context, req *connect.Request[likev1.UnlikeMatchupRequest]) (*connect.Response[likev1.UnlikeMatchupResponse], error) {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}
	matchupRecord, err := resolveMatchupByIdentifier(h.DB, req.Msg.MatchupId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("matchup not found"))
	}

	var existing models.Like
	if err := sqlx.GetContext(ctx, h.DB, &existing, "SELECT * FROM likes WHERE user_id = $1 AND matchup_id = $2", uid, matchupRecord.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("like not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if _, err := h.DB.ExecContext(ctx, "DELETE FROM likes WHERE id = $1", existing.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	var m models.Matchup
	if err := sqlx.GetContext(ctx, h.DB, &m, "SELECT * FROM matchups WHERE id = $1", matchupRecord.ID); err == nil {
		if m.BracketID != nil {
			invalidateBracketSummaryCache(*m.BracketID)
		}
		invalidateHomeSummaryCache(m.AuthorID)
	}
	resp := connect.NewResponse(&likev1.UnlikeMatchupResponse{Message: "matchup unliked"})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

func (h *LikeHandler) GetUserLikes(ctx context.Context, req *connect.Request[likev1.GetUserLikesRequest]) (*connect.Response[likev1.GetUserLikesResponse], error) {
	// Pure-read RPC — safe to serve from the replica.
	db := dbForRead(ctx, h.DB, h.ReadDB)

	user, err := resolveUserByIdentifier(db, req.Msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}
	var likes []models.Like
	if err := sqlx.SelectContext(ctx, db, &likes, "SELECT * FROM likes WHERE user_id = $1", user.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	matchupIDs := make([]uint, 0, len(likes))
	for _, l := range likes {
		matchupIDs = append(matchupIDs, l.MatchupID)
	}
	userPublicIDs := loadUserPublicIDMap(db, []uint{user.ID})
	matchupPublicIDs := loadMatchupPublicIDMap(db, matchupIDs)
	protos := make([]*likev1.LikeData, len(likes))
	for i, l := range likes {
		protos[i] = likeToProto(l, userPublicIDs[l.UserID], matchupPublicIDs[l.MatchupID])
	}
	return connect.NewResponse(&likev1.GetUserLikesResponse{Likes: protos}), nil
}

func (h *LikeHandler) GetBracketLikes(ctx context.Context, req *connect.Request[likev1.GetBracketLikesRequest]) (*connect.Response[likev1.GetBracketLikesResponse], error) {
	// Pure-read RPC — safe to serve from the replica.
	db := dbForRead(ctx, h.DB, h.ReadDB)

	bracketRecord, err := resolveBracketByIdentifier(db, req.Msg.BracketId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("bracket not found"))
	}
	var bracket models.Bracket
	if err := sqlx.GetContext(ctx, db, &bracket, "SELECT * FROM brackets WHERE id = $1", bracketRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("bracket not found"))
	}
	if err := sqlx.GetContext(ctx, db, &bracket.Author, "SELECT * FROM users WHERE id = $1", bracket.AuthorID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("bracket not found"))
	}
	bracket.Author.ProcessAvatarPath()

	viewerID, hasViewer := optionalViewerFromCtx(ctx)
	allowed, reason, err := canViewUserContent(db, viewerID, hasViewer, &bracket.Author, bracket.Visibility, httpctx.IsAdminRequest(ctx))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("%s", visibilityErrorMessage(reason)))
	}

	var likes []models.BracketLike
	if err := sqlx.SelectContext(ctx, db, &likes, "SELECT * FROM bracket_likes WHERE bracket_id = $1", bracketRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	userIDs := make([]uint, 0, len(likes))
	for _, l := range likes {
		userIDs = append(userIDs, l.UserID)
	}
	userPublicIDs := loadUserPublicIDMap(db, userIDs)
	protos := make([]*likev1.BracketLikeData, len(likes))
	for i, l := range likes {
		protos[i] = bracketLikeToProto(l, userPublicIDs[l.UserID], bracketRecord.PublicID)
	}
	return connect.NewResponse(&likev1.GetBracketLikesResponse{Likes: protos}), nil
}

func (h *LikeHandler) GetUserBracketLikes(ctx context.Context, req *connect.Request[likev1.GetUserBracketLikesRequest]) (*connect.Response[likev1.GetUserBracketLikesResponse], error) {
	// Pure-read RPC — safe to serve from the replica.
	db := dbForRead(ctx, h.DB, h.ReadDB)

	user, err := resolveUserByIdentifier(db, req.Msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}
	var likes []models.BracketLike
	if err := sqlx.SelectContext(ctx, db, &likes, "SELECT * FROM bracket_likes WHERE user_id = $1", user.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	bracketIDs := make([]uint, 0, len(likes))
	for _, l := range likes {
		bracketIDs = append(bracketIDs, l.BracketID)
	}
	bracketPublicIDs := loadBracketPublicIDMap(db, bracketIDs)
	protos := make([]*likev1.BracketLikeData, len(likes))
	for i, l := range likes {
		protos[i] = bracketLikeToProto(l, user.PublicID, bracketPublicIDs[l.BracketID])
	}
	return connect.NewResponse(&likev1.GetUserBracketLikesResponse{Likes: protos}), nil
}

func (h *LikeHandler) LikeBracket(ctx context.Context, req *connect.Request[likev1.LikeBracketRequest]) (*connect.Response[likev1.LikeBracketResponse], error) {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}
	bracketRecord, err := resolveBracketByIdentifier(h.DB, req.Msg.BracketId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("bracket not found"))
	}
	var bracket models.Bracket
	if err := sqlx.GetContext(ctx, h.DB, &bracket, "SELECT * FROM brackets WHERE id = $1", bracketRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("bracket not found"))
	}
	if err := sqlx.GetContext(ctx, h.DB, &bracket.Author, "SELECT * FROM users WHERE id = $1", bracket.AuthorID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("bracket not found"))
	}
	bracket.Author.ProcessAvatarPath()

	allowed, reason, err := canViewUserContent(h.DB, uid, true, &bracket.Author, bracket.Visibility, httpctx.IsAdminRequest(ctx))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("%s", visibilityErrorMessage(reason)))
	}

	var existing models.BracketLike
	err = sqlx.GetContext(ctx, h.DB, &existing, "SELECT * FROM bracket_likes WHERE user_id = $1 AND bracket_id = $2", uid, bracketRecord.ID)
	if err == nil {
		return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("already liked this bracket"))
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	like := models.BracketLike{
		PublicID:  appdb.GeneratePublicID(),
		UserID:    uid,
		BracketID: bracketRecord.ID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if _, err := h.DB.ExecContext(ctx,
		"INSERT INTO bracket_likes (public_id, user_id, bracket_id, created_at, updated_at) VALUES ($1, $2, $3, $4, $5)",
		like.PublicID, like.UserID, like.BracketID, like.CreatedAt, like.UpdatedAt); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	invalidateBracketSummaryCache(bracket.ID)
	invalidateHomeSummaryCache(bracket.AuthorID)

	userPublicID := resolveUserPublicID(h.DB, nil, uid)
	resp := connect.NewResponse(&likev1.LikeBracketResponse{Like: bracketLikeToProto(like, userPublicID, bracketRecord.PublicID)})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

func (h *LikeHandler) UnlikeBracket(ctx context.Context, req *connect.Request[likev1.UnlikeBracketRequest]) (*connect.Response[likev1.UnlikeBracketResponse], error) {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}
	bracketRecord, err := resolveBracketByIdentifier(h.DB, req.Msg.BracketId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("bracket not found"))
	}
	var existing models.BracketLike
	if err := sqlx.GetContext(ctx, h.DB, &existing, "SELECT * FROM bracket_likes WHERE user_id = $1 AND bracket_id = $2", uid, bracketRecord.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("like not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if _, err := h.DB.ExecContext(ctx, "DELETE FROM bracket_likes WHERE id = $1", existing.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	var bracket models.Bracket
	if err := sqlx.GetContext(ctx, h.DB, &bracket, "SELECT * FROM brackets WHERE id = $1", bracketRecord.ID); err == nil {
		invalidateBracketSummaryCache(bracket.ID)
		invalidateHomeSummaryCache(bracket.AuthorID)
	}
	resp := connect.NewResponse(&likev1.UnlikeBracketResponse{Message: "bracket unliked"})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}
