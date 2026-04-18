package controllers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"Matchup/auth"
	"Matchup/cache"
	appdb "Matchup/db"
	matchupv1 "Matchup/gen/matchup/v1"
	"Matchup/gen/matchup/v1/matchupv1connect"
	"Matchup/models"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// MatchupItemHandler implements matchupv1connect.MatchupItemServiceHandler.
type MatchupItemHandler struct {
	DB     *sqlx.DB
	ReadDB *sqlx.DB
}

var _ matchupv1connect.MatchupItemServiceHandler = (*MatchupItemHandler)(nil)

// resolveVoteIdentityFromRequest resolves a voteIdentity from a ConnectRPC request's HTTP headers.
// Caller supplies the raw http.Request so we can extract the token and user-agent/IP.
func resolveVoteIdentityFromRequest(r *http.Request, anonIDOverride *string, db sqlx.ExtContext) (voteIdentity, error) {
	if token := auth.ExtractToken(r); token != "" {
		uid, err := auth.ExtractTokenID(r)
		if err == nil {
			return voteIdentity{UserID: &uid}, nil
		}
	}
	// Use override (e.g., from request message field) or fall back to device fingerprint.
	if anonIDOverride != nil && *anonIDOverride != "" {
		if err := upsertAnonymousDevice(db, *anonIDOverride, hashFingerprint(r.UserAgent()), hashFingerprint(r.RemoteAddr)); err != nil {
			return voteIdentity{}, err
		}
		return voteIdentity{AnonID: anonIDOverride}, nil
	}
	// Build a deterministic anon ID from fingerprint.
	uaHash := hashFingerprint(r.UserAgent())
	ipHash := hashFingerprint(r.RemoteAddr)
	deviceID := ""
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	var existing models.AnonymousDevice
	if err := sqlx.GetContext(context.Background(), db, &existing,
		"SELECT * FROM anonymous_devices WHERE user_agent_hash = $1 AND ip_hash = $2 AND last_seen_at >= $3 ORDER BY last_seen_at DESC LIMIT 1",
		uaHash, ipHash, cutoff); err == nil {
		deviceID = existing.DeviceID
	}
	if deviceID == "" {
		deviceID = appdb.GeneratePublicID()
	}
	if err := upsertAnonymousDevice(db, deviceID, uaHash, ipHash); err != nil {
		return voteIdentity{}, err
	}
	return voteIdentity{AnonID: &deviceID}, nil
}

func (h *MatchupItemHandler) VoteItem(ctx context.Context, req *connect.Request[matchupv1.VoteItemRequest]) (*connect.Response[matchupv1.VoteItemResponse], error) {
	httpReq := &http.Request{Header: req.Header(), URL: &url.URL{}, RemoteAddr: req.Peer().Addr}
	identity, err := resolveVoteIdentityFromRequest(httpReq, req.Msg.AnonId, h.DB)
	if err != nil {
		if errors.Is(err, errInvalidAuthToken) {
			return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var item models.MatchupItem
	if err := sqlx.GetContext(ctx, h.DB, &item, "SELECT * FROM matchup_items WHERE public_id = $1", req.Msg.Id); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("matchup item not found"))
	}
	if err := sqlx.GetContext(ctx, h.DB, &item.Matchup, "SELECT * FROM matchups WHERE id = $1", item.MatchupID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("matchup not found"))
	}

	var owner models.User
	if err := sqlx.GetContext(ctx, h.DB, &owner, "SELECT * FROM users WHERE id = $1", item.Matchup.AuthorID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	viewerID := uint(0)
	hasViewer := false
	if identity.UserID != nil {
		viewerID = *identity.UserID
		hasViewer = true
	}
	allowed, reason, err := canViewUserContent(h.DB, viewerID, hasViewer, &owner, item.Matchup.Visibility, httpctx.IsAdminRequest(ctx))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("%s", visibilityErrorMessage(reason)))
	}

	isOwner := identity.UserID != nil && *identity.UserID == item.Matchup.AuthorID
	if !isOwner {
		if item.Matchup.Status == matchupStatusCompleted {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("voting is locked for this matchup"))
		}
		if item.Matchup.BracketID == nil && !isMatchupOpenStatus(item.Matchup.Status) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("matchup is not active"))
		}
		if item.Matchup.EndTime != nil && time.Now().After(*item.Matchup.EndTime) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("voting is closed for this matchup"))
		}
		if item.Matchup.BracketID != nil {
			var bracket models.Bracket
			if err := sqlx.GetContext(ctx, h.DB, &bracket, "SELECT * FROM brackets WHERE id = $1", *item.Matchup.BracketID); err != nil {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("bracket not found"))
			}
			if bracket.Status != "active" {
				return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("bracket is not active"))
			}
			if item.Matchup.Round == nil || *item.Matchup.Round != bracket.CurrentRound {
				return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("voting is locked for this round"))
			}
		}
	}

	var existingVote models.MatchupVote
	var voteErr error
	if identity.UserID != nil {
		voteErr = sqlx.GetContext(ctx, h.DB, &existingVote,
			"SELECT * FROM matchup_votes WHERE matchup_public_id = $1 AND user_id = $2",
			item.Matchup.PublicID, *identity.UserID)
	} else {
		voteErr = sqlx.GetContext(ctx, h.DB, &existingVote,
			"SELECT * FROM matchup_votes WHERE matchup_public_id = $1 AND anon_id = $2",
			item.Matchup.PublicID, *identity.AnonID)
	}
	if voteErr != nil && !errors.Is(voteErr, sql.ErrNoRows) {
		return nil, connect.NewError(connect.CodeInternal, voteErr)
	}

	// Track item-id deltas to apply to the Redis vote buffer once the
	// matchup_votes write commits. Doing the cache writes outside the
	// DB transaction means a Redis outage can never roll back a vote
	// — the vote row is the source of truth, the buffer is just a
	// performance optimization.
	var voteIncrItemID uint
	var voteDecrItemID uint

	if voteErr == nil {
		// Vote exists on same item — already voted
		if existingVote.MatchupItemPublicID == item.PublicID {
			var updatedItem models.MatchupItem
			if err := sqlx.GetContext(ctx, h.DB, &updatedItem, "SELECT * FROM matchup_items WHERE id = $1", item.ID); err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			// Even on a no-op, surface any pending Redis delta so
			// the response matches what other endpoints would show.
			applyVoteDeltaToItem(ctx, &updatedItem)
			resp := connect.NewResponse(&matchupv1.VoteItemResponse{
				Item:         matchupItemToProto(updatedItem),
				AlreadyVoted: true,
			})
			setReadPrimaryCookie(resp.Header())
			return resp, nil
		}
		// Switch vote — only the matchup_votes pointer move stays
		// in-transaction. The matchup_items counter changes happen
		// in Redis after commit (see below).
		tx, err := h.DB.Beginx()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		defer tx.Rollback()

		var prevItem models.MatchupItem
		if err := sqlx.GetContext(ctx, tx, &prevItem, "SELECT * FROM matchup_items WHERE public_id = $1", existingVote.MatchupItemPublicID); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE matchup_votes SET matchup_item_public_id = $1 WHERE id = $2", item.PublicID, existingVote.ID); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		if err := tx.Commit(); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		voteDecrItemID = prevItem.ID
		voteIncrItemID = item.ID
	} else {
		// New vote — only the matchup_votes INSERT stays in the
		// transaction. The counter increment lives in Redis.
		tx, err := h.DB.Beginx()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		defer tx.Rollback()

		now := time.Now()
		var userID *uint
		var anonID *string
		if identity.UserID != nil {
			userID = identity.UserID
		} else {
			anonID = identity.AnonID
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO matchup_votes (public_id, user_id, anon_id, matchup_public_id, matchup_item_public_id, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			appdb.GeneratePublicID(), userID, anonID, item.Matchup.PublicID, item.PublicID, now, now); err != nil {
			var pqErr *pq.Error
			if errors.As(err, &pqErr) && pqErr.Code == "23505" {
				return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("already voted for this matchup"))
			}
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		if err := tx.Commit(); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		voteIncrItemID = item.ID
	}

	// Apply the buffered counter changes in Redis. Failures here are
	// logged but never returned — the source-of-truth row in
	// matchup_votes is already committed, and the flush worker can
	// reconstruct any missing deltas later.
	if cache.Client != nil {
		if voteIncrItemID != 0 {
			key := voteDeltaKey(voteIncrItemID)
			if err := cache.Client.Incr(ctx, key).Err(); err != nil {
				log.Printf("vote: redis incr failed for item %d: %v", voteIncrItemID, err)
			}
			// Safety-net TTL: under volatile-lru, only keys with a TTL
			// are eviction-eligible. The flush worker zeros these every
			// 5s, so the TTL is a belt-and-suspenders guard against
			// stale keys pinning memory if the flush worker is behind.
			cache.Client.Expire(ctx, key, 2*time.Hour)
		}
		if voteDecrItemID != 0 {
			key := voteDeltaKey(voteDecrItemID)
			if err := cache.Client.Decr(ctx, key).Err(); err != nil {
				log.Printf("vote: redis decr failed for item %d: %v", voteDecrItemID, err)
			}
			cache.Client.Expire(ctx, key, 2*time.Hour)
		}
	} else {
		// Redis unavailable — fall back to direct DB writes so the
		// counts don't drift. This is the dev path; production
		// always has Redis.
		if voteIncrItemID != 0 {
			if _, err := h.DB.ExecContext(ctx, "UPDATE matchup_items SET votes = votes + 1 WHERE id = $1", voteIncrItemID); err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
		}
		if voteDecrItemID != 0 {
			if _, err := h.DB.ExecContext(ctx, "UPDATE matchup_items SET votes = GREATEST(votes - 1, 0) WHERE id = $1", voteDecrItemID); err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
		}
	}

	var updatedItem models.MatchupItem
	if err := sqlx.GetContext(ctx, h.DB, &updatedItem, "SELECT * FROM matchup_items WHERE id = $1", item.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// Merge any pending Redis delta so the response shows the live
	// count, not the (possibly stale) DB row.
	applyVoteDeltaToItem(ctx, &updatedItem)

	if item.Matchup.BracketID != nil {
		invalidateBracketSummaryCache(*item.Matchup.BracketID)
	}
	invalidateHomeSummaryCache(item.Matchup.AuthorID)

	resp := connect.NewResponse(&matchupv1.VoteItemResponse{Item: matchupItemToProto(updatedItem)})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

func (h *MatchupItemHandler) AddItem(ctx context.Context, req *connect.Request[matchupv1.AddItemRequest]) (*connect.Response[matchupv1.AddItemResponse], error) {
	requestorID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}
	matchupRecord, err := resolveMatchupByIdentifier(h.DB, req.Msg.MatchupId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("matchup not found"))
	}
	var m models.Matchup
	if err := sqlx.GetContext(ctx, h.DB, &m, "SELECT * FROM matchups WHERE id = $1", matchupRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("matchup not found"))
	}
	if m.AuthorID != requestorID && !httpctx.IsAdminRequest(ctx) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("unauthorized"))
	}

	item := models.MatchupItem{
		PublicID:  appdb.GeneratePublicID(),
		MatchupID: matchupRecord.ID,
		Item:      req.Msg.Item,
	}
	if _, err := h.DB.ExecContext(ctx,
		"INSERT INTO matchup_items (public_id, matchup_id, item, votes, created_at, updated_at) VALUES ($1, $2, $3, 0, NOW(), NOW())",
		item.PublicID, item.MatchupID, item.Item); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := sqlx.GetContext(ctx, h.DB, &item, "SELECT * FROM matchup_items WHERE public_id = $1", item.PublicID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if m.BracketID != nil {
		invalidateBracketSummaryCache(*m.BracketID)
	}
	resp := connect.NewResponse(&matchupv1.AddItemResponse{Item: matchupItemToProto(item)})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

func (h *MatchupItemHandler) UpdateItem(ctx context.Context, req *connect.Request[matchupv1.UpdateItemRequest]) (*connect.Response[matchupv1.UpdateItemResponse], error) {
	requestorID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}
	itemRecord, err := resolveMatchupItemByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("item not found"))
	}
	var m models.Matchup
	if err := sqlx.GetContext(ctx, h.DB, &m, "SELECT * FROM matchups WHERE id = $1", itemRecord.MatchupID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("matchup not found"))
	}
	if m.AuthorID != requestorID && !httpctx.IsAdminRequest(ctx) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("unauthorized"))
	}
	if _, err := h.DB.ExecContext(ctx, "UPDATE matchup_items SET item = $1, updated_at = NOW() WHERE id = $2", req.Msg.Item, itemRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := sqlx.GetContext(ctx, h.DB, itemRecord, "SELECT * FROM matchup_items WHERE id = $1", itemRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	resp := connect.NewResponse(&matchupv1.UpdateItemResponse{Item: matchupItemToProto(*itemRecord)})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

func (h *MatchupItemHandler) DeleteItem(ctx context.Context, req *connect.Request[matchupv1.DeleteItemRequest]) (*connect.Response[matchupv1.DeleteItemResponse], error) {
	requestorID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}
	itemRecord, err := resolveMatchupItemByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("item not found"))
	}
	var m models.Matchup
	if err := sqlx.GetContext(ctx, h.DB, &m, "SELECT * FROM matchups WHERE id = $1", itemRecord.MatchupID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("matchup not found"))
	}
	if m.AuthorID != requestorID && !httpctx.IsAdminRequest(ctx) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("unauthorized"))
	}
	if _, err := h.DB.ExecContext(ctx, "DELETE FROM matchup_items WHERE id = $1", itemRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if m.BracketID != nil {
		invalidateBracketSummaryCache(*m.BracketID)
	}
	resp := connect.NewResponse(&matchupv1.DeleteItemResponse{Message: "item deleted"})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

func (h *MatchupItemHandler) GetUserVotes(ctx context.Context, req *connect.Request[matchupv1.GetUserVotesRequest]) (*connect.Response[matchupv1.GetUserVotesResponse], error) {
	// Pure-read RPC — safe to serve from the replica.
	db := dbForRead(ctx, h.DB, h.ReadDB)

	user, err := resolveUserByIdentifier(db, req.Msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}
	var votes []models.MatchupVote
	if err := sqlx.SelectContext(ctx, db, &votes, "SELECT * FROM matchup_votes WHERE user_id = $1", user.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	protos := make([]*matchupv1.MatchupVoteData, len(votes))
	for i, v := range votes {
		p := user.PublicID
		protos[i] = &matchupv1.MatchupVoteData{
			Id:            v.PublicID,
			UserId:        &p,
			MatchupId:     v.MatchupPublicID,
			MatchupItemId: v.MatchupItemPublicID,
			CreatedAt:     rfc3339(v.CreatedAt),
		}
	}
	return connect.NewResponse(&matchupv1.GetUserVotesResponse{Votes: protos}), nil
}
