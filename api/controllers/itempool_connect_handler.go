package controllers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"Matchup/gen/common/v1"
	itempoolv1 "Matchup/gen/itempool/v1"
	"Matchup/gen/itempool/v1/itempoolv1connect"
	"Matchup/models"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
)

var _ itempoolv1connect.ItemPoolServiceHandler = (*ItemPoolHandler)(nil)

type ItemPoolHandler struct {
	DB     *sqlx.DB
	ReadDB *sqlx.DB
}

func itemPoolToProto(ip *models.ItemPool) *itempoolv1.ItemPoolData {
	data := &itempoolv1.ItemPoolData{
		Id:          ip.PublicID,
		Title:       ip.Title,
		Description: ip.Description,
		AuthorId:    ip.Author.PublicID,
		Size:        int32(ip.Size),
		Visibility:  ip.Visibility,
		CreatedAt:   rfc3339(ip.CreatedAt),
		UpdatedAt:   rfc3339(ip.UpdatedAt),
		Author: &commonv1.UserSummaryResponse{
			Id:       ip.Author.PublicID,
			Username: ip.Author.Username,
		},
	}
	if ip.ShortID != nil {
		data.ShortId = *ip.ShortID
	}
	
	for _, item := range ip.Items {
		data.Items = append(data.Items, &itempoolv1.ItemPoolItemData{
			Id:   fmt.Sprintf("%d", item.ID), // Simplified for item public ID, but typically we'd use a public ID or string ID
			Item: item.Item,
		})
	}
	
	return data
}

func (h *ItemPoolHandler) CreateItemPool(ctx context.Context, req *connect.Request[itempoolv1.CreateItemPoolRequest]) (*connect.Response[itempoolv1.CreateItemPoolResponse], error) {
	userID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}
	
	if err := requireVerifiedEmail(ctx, h.DB, userID); err != nil {
		return nil, err
	}

	ip := models.ItemPool{}
	ip.AuthorID = userID
	ip.Title = req.Msg.Title
	if req.Msg.Description != nil {
		ip.Description = *req.Msg.Description
	}
	if req.Msg.Visibility != nil {
		ip.Visibility = *req.Msg.Visibility
	}

	for _, it := range req.Msg.Items {
		ip.Items = append(ip.Items, models.ItemPoolItem{Item: it.Item})
	}

	ip.Prepare()
	if errs := ip.Validate(); len(errs) > 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("validation failed: %v", errs))
	}

	tx, err := h.DB.Beginx()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	createdIP, err := ip.SaveItemPool(tx)
	if err != nil {
		tx.Rollback()
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	resp := connect.NewResponse(&itempoolv1.CreateItemPoolResponse{
		ItemPool: itemPoolToProto(createdIP),
	})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

func (h *ItemPoolHandler) GetItemPool(ctx context.Context, req *connect.Request[itempoolv1.GetItemPoolRequest]) (*connect.Response[itempoolv1.GetItemPoolResponse], error) {
	// Simple identifier resolution: lookup by public_id or short_id
	var internalID uint
	err := sqlx.GetContext(ctx, h.DB, &internalID, "SELECT id FROM item_pools WHERE public_id = $1 OR short_id = $1 LIMIT 1", req.Msg.Id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("item pool not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	ip := &models.ItemPool{}
	if _, err := ip.FindItemPoolByID(h.DB, internalID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	
	// Visibility check
	viewerID, hasViewer := optionalViewerFromCtx(ctx)
	allowed, reason, err := canViewUserContent(h.DB, viewerID, hasViewer, &ip.Author, ip.Visibility, httpctx.IsAdminRequest(ctx))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New(visibilityErrorMessage(reason)))
	}

	return connect.NewResponse(&itempoolv1.GetItemPoolResponse{
		ItemPool: itemPoolToProto(ip),
	}), nil
}

func (h *ItemPoolHandler) GetUserItemPools(ctx context.Context, req *connect.Request[itempoolv1.GetUserItemPoolsRequest]) (*connect.Response[itempoolv1.GetUserItemPoolsResponse], error) {
	db := dbForRead(ctx, h.DB, h.ReadDB)

	owner, err := resolveUserByIdentifier(db, req.Msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	}

	viewerID, hasViewer := optionalViewerFromCtx(ctx)
	isAdmin := httpctx.IsAdminRequest(ctx)
	allowed, reason, err := canViewUserContent(db, viewerID, hasViewer, owner, visibilityPublic, isAdmin)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New(visibilityErrorMessage(reason)))
	}

	page := int(req.Msg.GetPage())
	if page < 1 {
		page = 1
	}
	limit := int(req.Msg.GetLimit())
	if limit < 1 || limit > 100 {
		limit = 10
	}
	offset := (page - 1) * limit

	var total int64
	if err := sqlx.GetContext(ctx, db, &total, "SELECT COUNT(*) FROM item_pools WHERE author_id = $1", owner.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var pools []models.ItemPool
	if err := sqlx.SelectContext(ctx, db, &pools,
		"SELECT * FROM item_pools WHERE author_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3",
		owner.ID, limit, offset); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if len(pools) > 0 {
		var author models.User
		if err := sqlx.GetContext(ctx, db, &author, "SELECT * FROM users WHERE id = $1", owner.ID); err == nil {
			author.ProcessAvatarPath()
			for i := range pools {
				pools[i].Author = author
			}
		}
	}

	var protoPools []*itempoolv1.ItemPoolData
	for i := range pools {
		ip := &pools[i]
		allowed, _, err := canViewUserContent(db, viewerID, hasViewer, owner, ip.Visibility, isAdmin)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		if !allowed {
			continue
		}
		protoPools = append(protoPools, itemPoolToProto(ip))
	}

	return connect.NewResponse(&itempoolv1.GetUserItemPoolsResponse{
		ItemPools:  protoPools,
		Pagination: paginationToProto(page, limit, total),
	}), nil
}

func (h *ItemPoolHandler) UpdateItemPool(ctx context.Context, req *connect.Request[itempoolv1.UpdateItemPoolRequest]) (*connect.Response[itempoolv1.UpdateItemPoolResponse], error) {
	requestorID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	var internalID uint
	err := sqlx.GetContext(ctx, h.DB, &internalID, "SELECT id FROM item_pools WHERE public_id = $1 OR short_id = $1 LIMIT 1", req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("item pool not found"))
	}

	ip := &models.ItemPool{}
	if _, err := ip.FindItemPoolByID(h.DB, internalID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if ip.AuthorID != requestorID && !httpctx.IsAdminRequest(ctx) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("not authorized"))
	}

	if req.Msg.Title != nil {
		ip.Title = *req.Msg.Title
	}
	if req.Msg.Description != nil {
		ip.Description = *req.Msg.Description
	}
	if req.Msg.Visibility != nil {
		ip.Visibility = *req.Msg.Visibility
	}
	
	if req.Msg.Items != nil {
		ip.Items = []models.ItemPoolItem{}
		for _, it := range req.Msg.Items {
			ip.Items = append(ip.Items, models.ItemPoolItem{Item: it.Item})
		}
	}

	ip.Prepare()
	if errs := ip.Validate(); len(errs) > 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("validation failed: %v", errs))
	}

	tx, err := h.DB.Beginx()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	updatedIP, err := ip.UpdateItemPool(tx)
	if err != nil {
		tx.Rollback()
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	resp := connect.NewResponse(&itempoolv1.UpdateItemPoolResponse{
		ItemPool: itemPoolToProto(updatedIP),
	})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

func (h *ItemPoolHandler) DeleteItemPool(ctx context.Context, req *connect.Request[itempoolv1.DeleteItemPoolRequest]) (*connect.Response[itempoolv1.DeleteItemPoolResponse], error) {
	requestorID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	var internalID uint
	var authorID uint
	err := h.DB.QueryRowContext(ctx, "SELECT id, author_id FROM item_pools WHERE public_id = $1 OR short_id = $1 LIMIT 1", req.Msg.Id).Scan(&internalID, &authorID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("item pool not found"))
	}

	if authorID != requestorID && !httpctx.IsAdminRequest(ctx) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("not authorized"))
	}

	ip := &models.ItemPool{}
	if _, err := ip.DeleteItemPool(h.DB, internalID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	resp := connect.NewResponse(&itempoolv1.DeleteItemPoolResponse{Message: "item pool deleted"})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}
