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
	"Matchup/middlewares"
	"Matchup/models"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// AnonVoteCap is the maximum number of vote ROWS a single anon UUID
// can hold across all matchups. Counting live rows (not events) means
// switching a vote between items on the same matchup doesn't burn a
// slot — only fresh votes on new matchups do. A deleted matchup also
// frees the slot back up, which falls out of the live-row count for
// free.
//
// Exposed as a const so the GetAnonVoteStatus RPC can return the same
// number the cap enforces — single source of truth.
const AnonVoteCap = 3

// anonVoteIPBudget is the per-hour cap on anon votes from a single
// IP, layered on top of the per-anon-id cap. Stops a single machine
// from churning fresh anon UUIDs in a loop.
const anonVoteIPBudget = 10

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

	// Anonymous-voter gates. None of these apply to authed users — a
	// signed-in account can vote freely (subject to bracket-round
	// rules below). The rejections here are mapped to dedicated UI
	// states on the frontend (sign-up CTA / bracket gate).
	isAnon := identity.UserID == nil
	if isAnon {
		// Brackets are members-only. Bracket child matchups always
		// have BracketID set; standalone matchups don't.
		if item.Matchup.BracketID != nil {
			return nil, connect.NewError(
				connect.CodePermissionDenied,
				fmt.Errorf("create an account to vote in brackets"),
			)
		}
		// IP-tier rate limit. Layered ABOVE the per-anon-id cap so
		// localStorage-clear loops still pay a cost. Fail-open if
		// Redis is down; the per-anon cap below still gates abuse.
		if allowed, _ := middlewares.CheckIPRateLimit(
			ctx, req.Peer().Addr, "anon_vote_ip", anonVoteIPBudget, time.Hour,
		); !allowed {
			return nil, connect.NewError(
				connect.CodeResourceExhausted,
				fmt.Errorf("too many anon votes from this network; try again later"),
			)
		}
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
		if existingVote.PickedItemID() == item.PublicID {
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
		// in Redis after commit (see below). Setting kind back to
		// 'pick' here covers the skip → pick switch (user skipped,
		// then changed their mind and picked a contender).
		tx, err := h.DB.Beginx()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		defer tx.Rollback()

		// Only fetch the previous item if the existing vote was a
		// pick. Skip rows have a nil item reference, so there's no
		// previous item to decrement — voteDecrItemID stays zero
		// in that branch.
		if prev := existingVote.PickedItemID(); prev != "" {
			var prevItem models.MatchupItem
			if err := sqlx.GetContext(ctx, tx, &prevItem, "SELECT * FROM matchup_items WHERE public_id = $1", prev); err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			voteDecrItemID = prevItem.ID
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE matchup_votes SET matchup_item_public_id = $1, kind = 'pick', updated_at = NOW() WHERE id = $2",
			item.PublicID, existingVote.ID,
		); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		if err := tx.Commit(); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		voteIncrItemID = item.ID
	} else {
		// New vote — only the matchup_votes INSERT stays in the
		// transaction. The counter increment lives in Redis.
		//
		// Per-anon cap fires HERE, on the new-vote path only. The
		// switch-vote branch above just moves an existing row to a
		// different item, so it doesn't add to the count and shouldn't
		// be gated. Counting live rows (rather than tracking a
		// counter) means a deleted matchup naturally restores a slot.
		if isAnon {
			// Skip rows don't count toward the anon cap (migration 025
			// added the kind column). Picks are the gated event — skips
			// are intended to be free so users can browse without
			// burning their 3 free votes on "I can't decide".
			var used int
			if err := h.DB.GetContext(ctx, &used,
				`SELECT COUNT(*) FROM matchup_votes WHERE anon_id = $1 AND kind = 'pick'`,
				*identity.AnonID,
			); err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			if used >= AnonVoteCap {
				return nil, connect.NewError(
					connect.CodeResourceExhausted,
					fmt.Errorf("free vote limit reached"),
				)
			}
		}

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

	// SSE push — the matchup author should get a fresh bell state
	// without waiting on the poll. Skip self-votes (author_id == voter)
	// and anonymous votes (no viewer bell to push to). The voter's own
	// feed refetches too so vote_cast surfaces near-instantly.
	voterID := uint(0)
	if identity.UserID != nil {
		voterID = *identity.UserID
	}
	if voterID != 0 && voterID != item.Matchup.AuthorID {
		_ = cache.PublishActivity(ctx, item.Matchup.AuthorID)
	}
	if voterID != 0 {
		_ = cache.PublishActivity(ctx, voterID)
	}

	resp := connect.NewResponse(&matchupv1.VoteItemResponse{Item: matchupItemToProto(updatedItem)})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

// SkipMatchup records that the viewer chose not to pick either contender
// on a matchup. The skip lands in matchup_votes alongside picks but with
// kind='skip' and a NULL matchup_item_public_id (migration 025 added the
// column + nullability + check constraints). Skips do NOT count toward
// the anon vote cap and do NOT increment any item's votes counter.
//
// Three flows:
//
//  1. No prior vote on this matchup → INSERT a fresh skip row.
//  2. Prior pick on this matchup → UPDATE the row to skip + decrement the
//     previously-picked item's counter (same ledger semantics as switching
//     between picks).
//  3. Prior skip on this matchup → idempotent no-op; return AlreadySkipped.
//
// Anon callers can skip standalone matchups but not bracket matchups —
// brackets stay members-only, mirroring the VoteItem rule.
func (h *MatchupItemHandler) SkipMatchup(ctx context.Context, req *connect.Request[matchupv1.SkipMatchupRequest]) (*connect.Response[matchupv1.SkipMatchupResponse], error) {
	httpReq := &http.Request{Header: req.Header(), URL: &url.URL{}, RemoteAddr: req.Peer().Addr}
	identity, err := resolveVoteIdentityFromRequest(httpReq, req.Msg.AnonId, h.DB)
	if err != nil {
		if errors.Is(err, errInvalidAuthToken) {
			return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var m models.Matchup
	if err := sqlx.GetContext(ctx, h.DB, &m, "SELECT * FROM matchups WHERE public_id = $1", req.Msg.MatchupId); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("matchup not found"))
	}

	var owner models.User
	if err := sqlx.GetContext(ctx, h.DB, &owner, "SELECT * FROM users WHERE id = $1", m.AuthorID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	viewerID := uint(0)
	hasViewer := false
	if identity.UserID != nil {
		viewerID = *identity.UserID
		hasViewer = true
	}
	allowed, reason, err := canViewUserContent(h.DB, viewerID, hasViewer, &owner, m.Visibility, httpctx.IsAdminRequest(ctx))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("%s", visibilityErrorMessage(reason)))
	}

	isAnon := identity.UserID == nil
	if isAnon {
		if m.BracketID != nil {
			return nil, connect.NewError(
				connect.CodePermissionDenied,
				fmt.Errorf("create an account to vote in brackets"),
			)
		}
		// Anon skips ride the same IP-tier rate limiter as picks. A
		// machine churning anon UUIDs to spam skips is still abuse,
		// even if the cap doesn't apply.
		if allowed, _ := middlewares.CheckIPRateLimit(
			ctx, req.Peer().Addr, "anon_vote_ip", anonVoteIPBudget, time.Hour,
		); !allowed {
			return nil, connect.NewError(
				connect.CodeResourceExhausted,
				fmt.Errorf("too many anon actions from this network; try again later"),
			)
		}
	}

	isOwner := identity.UserID != nil && *identity.UserID == m.AuthorID
	if !isOwner {
		if m.Status == matchupStatusCompleted {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("voting is locked for this matchup"))
		}
		if m.BracketID == nil && !isMatchupOpenStatus(m.Status) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("matchup is not active"))
		}
		if m.EndTime != nil && time.Now().After(*m.EndTime) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("voting is closed for this matchup"))
		}
		if m.BracketID != nil {
			var bracket models.Bracket
			if err := sqlx.GetContext(ctx, h.DB, &bracket, "SELECT * FROM brackets WHERE id = $1", *m.BracketID); err != nil {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("bracket not found"))
			}
			if bracket.Status != "active" {
				return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("bracket is not active"))
			}
			if m.Round == nil || *m.Round != bracket.CurrentRound {
				return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("voting is locked for this round"))
			}
		}
	}

	var existingVote models.MatchupVote
	var voteErr error
	if identity.UserID != nil {
		voteErr = sqlx.GetContext(ctx, h.DB, &existingVote,
			"SELECT * FROM matchup_votes WHERE matchup_public_id = $1 AND user_id = $2",
			m.PublicID, *identity.UserID)
	} else {
		voteErr = sqlx.GetContext(ctx, h.DB, &existingVote,
			"SELECT * FROM matchup_votes WHERE matchup_public_id = $1 AND anon_id = $2",
			m.PublicID, *identity.AnonID)
	}
	if voteErr != nil && !errors.Is(voteErr, sql.ErrNoRows) {
		return nil, connect.NewError(connect.CodeInternal, voteErr)
	}

	var voteDecrItemID uint

	if voteErr == nil {
		// Already a vote on this matchup.
		if existingVote.Kind == "skip" {
			// Idempotent — return AlreadySkipped without touching the row.
			return connect.NewResponse(&matchupv1.SkipMatchupResponse{
				AlreadySkipped: true,
			}), nil
		}
		// Pick → skip. Decrement the previously-picked item's counter
		// and rewrite the row to a skip.
		if prev := existingVote.PickedItemID(); prev != "" {
			var prevItem models.MatchupItem
			if err := sqlx.GetContext(ctx, h.DB, &prevItem,
				"SELECT * FROM matchup_items WHERE public_id = $1", prev,
			); err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			voteDecrItemID = prevItem.ID
		}
		if _, err := h.DB.ExecContext(ctx,
			"UPDATE matchup_votes SET kind = 'skip', matchup_item_public_id = NULL, updated_at = NOW() WHERE id = $1",
			existingVote.ID,
		); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	} else {
		// No prior vote — insert a fresh skip. No anon-cap check
		// because skips never count.
		now := time.Now()
		var userID *uint
		var anonID *string
		if identity.UserID != nil {
			userID = identity.UserID
		} else {
			anonID = identity.AnonID
		}
		if _, err := h.DB.ExecContext(ctx,
			`INSERT INTO matchup_votes (public_id, user_id, anon_id, matchup_public_id, matchup_item_public_id, kind, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, NULL, 'skip', $5, $6)`,
			appdb.GeneratePublicID(), userID, anonID, m.PublicID, now, now,
		); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}

	// Apply the decrement (if any) to the Redis vote buffer; fall back
	// to a direct DB write when Redis is unavailable. Same belt+
	// suspenders dance as VoteItem.
	if voteDecrItemID != 0 {
		if cache.Client != nil {
			key := voteDeltaKey(voteDecrItemID)
			if err := cache.Client.Decr(ctx, key).Err(); err != nil {
				log.Printf("skip: redis decr failed for item %d: %v", voteDecrItemID, err)
			}
			cache.Client.Expire(ctx, key, 2*time.Hour)
		} else {
			if _, err := h.DB.ExecContext(ctx,
				"UPDATE matchup_items SET votes = GREATEST(votes - 1, 0) WHERE id = $1",
				voteDecrItemID,
			); err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
		}
	}

	if m.BracketID != nil {
		invalidateBracketSummaryCache(*m.BracketID)
	}
	invalidateHomeSummaryCache(m.AuthorID)

	resp := connect.NewResponse(&matchupv1.SkipMatchupResponse{AlreadySkipped: false})
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

// GetAnonVoteStatus returns how many votes the given anon UUID has
// cast across all matchups, along with the cap. Frontend uses this
// to render the "X of 3 free votes left" counter without duplicating
// the cap constant. Empty anon_id is treated as "no votes used yet"
// rather than an error so the home page can call this on first load
// before localStorage has been touched.
func (h *MatchupItemHandler) GetAnonVoteStatus(ctx context.Context, req *connect.Request[matchupv1.GetAnonVoteStatusRequest]) (*connect.Response[matchupv1.GetAnonVoteStatusResponse], error) {
	anonID := req.Msg.GetAnonId()
	used := int32(0)
	if anonID != "" {
		// Pure read — safe on the replica.
		// Filter by kind='pick' — only picks count toward the cap;
		// see VoteItem for the matching enforcement query (migration 025).
		db := dbForRead(ctx, h.DB, h.ReadDB)
		var n int
		if err := sqlx.GetContext(ctx, db, &n,
			`SELECT COUNT(*) FROM matchup_votes WHERE anon_id = $1 AND kind = 'pick'`,
			anonID,
		); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		used = int32(n)
	}
	return connect.NewResponse(&matchupv1.GetAnonVoteStatusResponse{
		Used: used,
		Max:  int32(AnonVoteCap),
	}), nil
}

func (h *MatchupItemHandler) GetUserVotes(ctx context.Context, req *connect.Request[matchupv1.GetUserVotesRequest]) (*connect.Response[matchupv1.GetUserVotesResponse], error) {
	// Pure-read RPC — safe to serve from the replica.
	db := dbForRead(ctx, h.DB, h.ReadDB)

	user, err := resolveUserByIdentifier(db, req.Msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}
	// Filter to picks only — GetUserVotes powers the "matchups I voted
	// on" UI, where skips would surface as "voted: nothing", which
	// is confusing. Skip aggregates are exposed elsewhere via the
	// per-matchup tally queries.
	var votes []models.MatchupVote
	if err := sqlx.SelectContext(ctx, db, &votes,
		"SELECT * FROM matchup_votes WHERE user_id = $1 AND kind = 'pick'", user.ID,
	); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	protos := make([]*matchupv1.MatchupVoteData, len(votes))
	for i, v := range votes {
		p := user.PublicID
		protos[i] = &matchupv1.MatchupVoteData{
			Id:            v.PublicID,
			UserId:        &p,
			MatchupId:     v.MatchupPublicID,
			MatchupItemId: v.PickedItemID(),
			CreatedAt:     rfc3339(v.CreatedAt),
		}
	}
	return connect.NewResponse(&matchupv1.GetUserVotesResponse{Votes: protos}), nil
}
