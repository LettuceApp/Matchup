package controllers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"Matchup/cache"
	appdb "Matchup/db"
	bracketv1 "Matchup/gen/bracket/v1"
	"Matchup/gen/bracket/v1/bracketv1connect"
	matchupv1 "Matchup/gen/matchup/v1"
	"Matchup/models"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// tryAdvanceIfExpired is the request-path "safety net" advance. Unlike
// the scheduler (which runs every 10s), this fires only when a reader
// notices a bracket's round is overdue. Two layers of defense prevent
// this from creating lock contention with the scheduler or other readers:
//
//  1. Cheap in-memory pre-check on the bracket struct already loaded —
//     eliminates 99%+ of calls without touching the DB or Redis.
//  2. Redis SETNX distributed lock — only one goroutine per bracket
//     enters the expensive FOR UPDATE path. The 30s TTL acts as a
//     cooldown between attempts.
func (h *BracketHandler) tryAdvanceIfExpired(ctx context.Context, bracket *models.Bracket) {
	if bracket.Status != "active" ||
		bracket.AdvanceMode != "timer" ||
		bracket.RoundEndsAt == nil ||
		time.Now().Before(*bracket.RoundEndsAt) {
		return
	}

	if cache.Client != nil {
		lockKey := fmt.Sprintf("advance_lock:%d", bracket.ID)
		ok, _ := cache.Client.SetNX(ctx, lockKey, "1", 30*time.Second).Result()
		if !ok {
			return
		}
		// Don't defer Del — let the key expire so there's a cooldown.
	}

	s := &Server{DB: h.DB}
	if _, err := s.advanceBracketInternal(h.DB, bracket); err != nil {
		log.Printf("tryAdvanceIfExpired bracket %d: %v", bracket.ID, err)
		if cache.Client != nil {
			cache.Client.Del(ctx, fmt.Sprintf("advance_lock:%d", bracket.ID))
		}
	}
}

var _ bracketv1connect.BracketServiceHandler = (*BracketHandler)(nil)

// BracketHandler implements bracketv1connect.BracketServiceHandler.
type BracketHandler struct {
	DB     *sqlx.DB
	ReadDB *sqlx.DB
}

func (h *BracketHandler) GetPopularBrackets(ctx context.Context, req *connect.Request[bracketv1.GetPopularBracketsRequest]) (*connect.Response[bracketv1.GetPopularBracketsResponse], error) {
	// Pure-read RPC — safe to serve from the replica.
	db := dbForRead(ctx, h.DB, h.ReadDB)

	const limit = 5
	viewerID, hasViewer := optionalViewerFromCtx(ctx)
	isAdmin := httpctx.IsAdminRequest(ctx)
	// Anon viewers can't see brackets at all — they browse + vote on
	// matchups, brackets are members-only. Reject early so the read
	// never hits the materialized view.
	if !hasViewer {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("create an account to view brackets"))
	}

	var rows []popularBracketRow
	if err := sqlx.SelectContext(ctx, db, &rows,
		"SELECT * FROM popular_brackets_snapshot ORDER BY rank ASC LIMIT $1", limit); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	bracketIDs := make([]uint, 0, len(rows))
	authorIDs := make([]uint, 0, len(rows))
	for _, row := range rows {
		bracketIDs = append(bracketIDs, row.ID)
		authorIDs = append(authorIDs, row.AuthorID)
	}

	bracketPublicIDs := loadBracketPublicIDMap(db, bracketIDs)
	authorPublicIDs := loadUserPublicIDMap(db, authorIDs)

	bracketsMap := getBracketsByIDs(db, bracketIDs)
	authorsMap := getUsersByIDs(db, authorIDs)

	var protoBrackets []*bracketv1.PopularBracketData
	for i := range rows {
		bracket, ok := bracketsMap[rows[i].ID]
		if !ok {
			continue
		}
		author, ok := authorsMap[bracket.AuthorID]
		if !ok {
			continue
		}
		bracket.Author = *author

		allowed, _, err := canViewUserContent(db, viewerID, hasViewer, &bracket.Author, bracket.Visibility, isAdmin)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		if !allowed || bracket.Status != "active" {
			continue
		}

		dto := PopularBracketDTO{
			ID:              bracketPublicIDs[rows[i].ID],
			Title:           rows[i].Title,
			AuthorID:        authorPublicIDs[rows[i].AuthorID],
			AuthorUsername:  bracket.Author.Username,
			CurrentRound:    rows[i].CurrentRound,
			Size:            bracket.Size,
			Votes:           0,
			Likes:           int64(bracket.LikesCount),
			Comments:        int64(bracket.CommentsCount),
			EngagementScore: rows[i].EngagementScore,
			Rank:            rows[i].Rank,
			CreatedAt:       rfc3339(bracket.CreatedAt),
		}
		protoBrackets = append(protoBrackets, popularBracketToProto(dto))
	}

	return connect.NewResponse(&bracketv1.GetPopularBracketsResponse{
		Brackets: protoBrackets,
	}), nil
}

func (h *BracketHandler) GetBracket(ctx context.Context, req *connect.Request[bracketv1.GetBracketRequest]) (*connect.Response[bracketv1.GetBracketResponse], error) {
	// Members-only — anon can't view brackets. Frontend guards the route
	// too but the backend rejection is the source of truth.
	if _, hasViewer := optionalViewerFromCtx(ctx); !hasViewer {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("create an account to view brackets"))
	}
	bracketRecord, err := resolveBracketByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		} else if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var bracket models.Bracket
	found, err := bracket.FindBracketByID(h.DB, bracketRecord.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
	}

	viewerID, hasViewer := optionalViewerFromCtx(ctx)
	allowed, reason, err := canViewUserContent(h.DB, viewerID, hasViewer, &found.Author, found.Visibility, httpctx.IsAdminRequest(ctx))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New(visibilityErrorMessage(reason)))
	}

	h.tryAdvanceIfExpired(ctx, found)
	// Re-read to reflect any advance that just occurred.
	if refreshed, err := bracket.FindBracketByID(h.DB, found.ID); err == nil {
		found = refreshed
	}

	// LikesCount is loaded from the row directly via FindBracketByID (migration 006).
	return connect.NewResponse(&bracketv1.GetBracketResponse{
		Bracket: bracketToProto(h.DB, found),
	}), nil
}

func (h *BracketHandler) GetBracketSummary(ctx context.Context, req *connect.Request[bracketv1.GetBracketSummaryRequest]) (*connect.Response[bracketv1.GetBracketSummaryResponse], error) {
	// Members-only (see GetBracket).
	if _, hasViewer := optionalViewerFromCtx(ctx); !hasViewer {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("create an account to view brackets"))
	}
	bracketRecord, err := resolveBracketByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		} else if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	viewerID, hasViewer := optionalViewerFromCtx(ctx)

	var bracket models.Bracket
	found, err := bracket.FindBracketByID(h.DB, bracketRecord.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
	}
	bracket = *found

	allowed, reason, err := canViewUserContent(h.DB, viewerID, hasViewer, &bracket.Author, bracket.Visibility, httpctx.IsAdminRequest(ctx))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New(visibilityErrorMessage(reason)))
	}

	h.tryAdvanceIfExpired(ctx, &bracket)

	matchups, err := models.FindMatchupsByBracket(h.DB, bracketRecord.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	for i := range matchups {
		if err := dedupeBracketMatchupItems(h.DB, &matchups[i]); err != nil {
			log.Printf("dedupe bracket matchup %d: %v", matchups[i].ID, err)
		}
	}
	// Live vote counts: merge any pending Redis deltas before mapping
	// the matchup items into the proto response. One MGET covers every
	// item across every matchup in the bracket.
	applyVoteDeltasToMatchups(ctx, matchups)

	likedMatchupIDs := []uint{}
	likedBracket := false

	if viewerID > 0 {
		if err := sqlx.SelectContext(ctx, h.DB, &likedMatchupIDs,
			"SELECT matchup_id FROM likes WHERE user_id = $1 AND matchup_id IN (SELECT id FROM matchups WHERE bracket_id = $2)",
			viewerID, bracketRecord.ID); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		var count int64
		if err := sqlx.GetContext(ctx, h.DB, &count,
			"SELECT COUNT(*) FROM bracket_likes WHERE user_id = $1 AND bracket_id = $2",
			viewerID, bracketRecord.ID); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		likedBracket = count > 0
	}
	// bracket.LikesCount comes directly from the row (migration 006).

	var protoMatchups []*matchupv1.MatchupData
	matchupPublicIDs := make(map[uint]string, len(matchups))
	for i := range matchups {
		protoMatchups = append(protoMatchups, matchupToProto(h.DB, &matchups[i], nil))
		matchupPublicIDs[matchups[i].ID] = matchups[i].PublicID
	}

	likedPublicIDs := make([]string, 0, len(likedMatchupIDs))
	for _, id := range likedMatchupIDs {
		if publicID, ok := matchupPublicIDs[id]; ok {
			likedPublicIDs = append(likedPublicIDs, publicID)
		}
	}

	return connect.NewResponse(&bracketv1.GetBracketSummaryResponse{
		Summary: &bracketv1.BracketSummaryData{
			Bracket:         bracketToProto(h.DB, &bracket),
			Matchups:        protoMatchups,
			LikedMatchupIds: likedPublicIDs,
			LikedBracket:    likedBracket,
		},
	}), nil
}

func (h *BracketHandler) GetUserBrackets(ctx context.Context, req *connect.Request[bracketv1.GetUserBracketsRequest]) (*connect.Response[bracketv1.GetUserBracketsResponse], error) {
	// Pure-read RPC — safe to serve from the replica.
	db := dbForRead(ctx, h.DB, h.ReadDB)

	// Members-only — anon can't see anyone's bracket list. Profile pages
	// for anon will see matchups + likes; the brackets tab on a profile
	// surfaces "Sign up to see brackets" when this returns Unauthenticated.
	if _, hasViewer := optionalViewerFromCtx(ctx); !hasViewer {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("create an account to view brackets"))
	}

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
		limit = 12
	}
	offset := (page - 1) * limit

	var total int64
	if err := sqlx.GetContext(ctx, db, &total, "SELECT COUNT(*) FROM brackets WHERE author_id = $1", owner.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var brackets []models.Bracket
	if err := sqlx.SelectContext(ctx, db, &brackets,
		"SELECT * FROM brackets WHERE author_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3",
		owner.ID, limit, offset); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Load author once for all brackets (all share the same owner)
	if len(brackets) > 0 {
		var author models.User
		if err := sqlx.GetContext(ctx, db, &author, "SELECT * FROM users WHERE id = $1", owner.ID); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		author.ProcessAvatarPath()
		for i := range brackets {
			brackets[i].Author = author
		}
	}

	// LikesCount lives on the row (migration 006).
	var protoBrackets []*bracketv1.BracketData
	for i := range brackets {
		b := &brackets[i]
		allowed, _, err := canViewUserContent(db, viewerID, hasViewer, owner, b.Visibility, isAdmin)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		if !allowed {
			continue
		}
		protoBrackets = append(protoBrackets, bracketToProto(db, b))
	}

	return connect.NewResponse(&bracketv1.GetUserBracketsResponse{
		Brackets:   protoBrackets,
		Pagination: paginationToProto(page, limit, total),
	}), nil
}

func (h *BracketHandler) GetBracketMatchups(ctx context.Context, req *connect.Request[bracketv1.GetBracketMatchupsRequest]) (*connect.Response[bracketv1.GetBracketMatchupsResponse], error) {
	// Members-only (see GetBracket).
	if _, hasViewer := optionalViewerFromCtx(ctx); !hasViewer {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("create an account to view brackets"))
	}
	bracketRecord, err := resolveBracketByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		} else if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var bracket models.Bracket
	found, err := bracket.FindBracketByID(h.DB, bracketRecord.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
	}

	viewerID, hasViewer := optionalViewerFromCtx(ctx)
	allowed, reason, err := canViewUserContent(h.DB, viewerID, hasViewer, &found.Author, found.Visibility, httpctx.IsAdminRequest(ctx))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New(visibilityErrorMessage(reason)))
	}

	h.tryAdvanceIfExpired(ctx, found)

	page := int(req.Msg.GetPage())
	if page < 1 {
		page = 1
	}
	pageLimit := int(req.Msg.GetLimit())
	if pageLimit < 1 || pageLimit > 100 {
		pageLimit = 12
	}
	offset := (page - 1) * pageLimit

	var total int64
	if err := sqlx.GetContext(ctx, h.DB, &total, "SELECT COUNT(*) FROM matchups WHERE bracket_id = $1", bracketRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var matchups []models.Matchup
	if err := sqlx.SelectContext(ctx, h.DB, &matchups,
		"SELECT * FROM matchups WHERE bracket_id = $1 ORDER BY round ASC, seed ASC, created_at ASC LIMIT $2 OFFSET $3",
		bracketRecord.ID, pageLimit, offset); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := models.LoadMatchupAssociations(h.DB, matchups); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	for i := range matchups {
		if err := dedupeBracketMatchupItems(h.DB, &matchups[i]); err != nil {
			log.Printf("dedupe bracket matchup %d: %v", matchups[i].ID, err)
		}
	}
	// Merge live vote deltas before serializing — one MGET per response.
	applyVoteDeltasToMatchups(ctx, matchups)

	// LikesCount lives on each matchup row (migration 006).
	protoMatchups := make([]*matchupv1.MatchupData, 0, len(matchups))
	for i := range matchups {
		protoMatchups = append(protoMatchups, matchupToProto(h.DB, &matchups[i], nil))
	}

	return connect.NewResponse(&bracketv1.GetBracketMatchupsResponse{
		Matchups:   protoMatchups,
		Pagination: paginationToProto(page, pageLimit, total),
	}), nil
}

func (h *BracketHandler) CreateBracket(ctx context.Context, req *connect.Request[bracketv1.CreateBracketRequest]) (*connect.Response[bracketv1.CreateBracketResponse], error) {
	userID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}
	// Email-verification soft-gate. See matchup_connect_handler.go.
	if err := requireVerifiedEmail(ctx, h.DB, userID); err != nil {
		return nil, err
	}

	roundDurationSeconds := 0
	if req.Msg.RoundDurationSeconds != nil {
		roundDurationSeconds = int(*req.Msg.RoundDurationSeconds)
	} else if req.Msg.DurationMinutes != nil {
		roundDurationSeconds = int(*req.Msg.DurationMinutes) * 60
	}
	if roundDurationSeconds <= 0 {
		roundDurationSeconds = defaultRoundDurationSeconds
	}

	advanceMode := "manual"
	if req.Msg.AdvanceMode != nil && *req.Msg.AdvanceMode != "" {
		advanceMode = *req.Msg.AdvanceMode
	}

	visibility := visibilityPublic
	if req.Msg.Visibility != nil {
		visibility = normalizeVisibility(*req.Msg.Visibility)
	}

	bracket := models.Bracket{
		Title:                req.Msg.Title,
		AuthorID:             userID,
		Size:                 int(req.Msg.Size),
		CurrentRound:         1,
		Status:               "draft",
		AdvanceMode:          advanceMode,
		RoundDurationSeconds: roundDurationSeconds,
		Visibility:           visibility,
		Tags:                 pq.StringArray(req.Msg.Tags),
	}
	if bracket.Tags == nil {
		bracket.Tags = pq.StringArray{}
	}
	if req.Msg.Description != nil {
		bracket.Description = *req.Msg.Description
	}

	if len(req.Msg.Entries) > 0 && len(req.Msg.Entries) != int(req.Msg.Size) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("entries must match bracket size"))
	}

	bracket.Prepare()

	// Up to 3 attempts: if the random short_id happens to collide with
	// an existing row (astronomically unlikely at 62^8 but not
	// impossible), Postgres rejects the INSERT with SQLSTATE 23505 and
	// we retry with a fresh ID. Anything else is fatal.
	var newBracket *models.Bracket
	var saveErr error
	for attempt := 0; attempt < 3; attempt++ {
		sid := appdb.GenerateShortID()
		bracket.ShortID = &sid
		newBracket, saveErr = bracket.SaveBracket(h.DB)
		if saveErr == nil {
			break
		}
		if !appdb.IsUniqueViolation(saveErr) {
			return nil, connect.NewError(connect.CodeInternal, saveErr)
		}
		log.Printf("short_id collision on bracket create (attempt %d), retrying", attempt+1)
	}
	if saveErr != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("short_id generation exhausted"))
	}

	generateFullBracket(h.DB, *newBracket, req.Msg.Entries)

	resp := connect.NewResponse(&bracketv1.CreateBracketResponse{
		Bracket: bracketToProto(h.DB, newBracket),
	})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

func (h *BracketHandler) UpdateBracket(ctx context.Context, req *connect.Request[bracketv1.UpdateBracketRequest]) (*connect.Response[bracketv1.UpdateBracketResponse], error) {
	userID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	bracketRecord, err := resolveBracketByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		} else if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var bracket models.Bracket
	if err := sqlx.GetContext(ctx, h.DB, &bracket, "SELECT * FROM brackets WHERE id = $1", bracketRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
	}

	if bracket.AuthorID != userID && !httpctx.IsAdminRequest(ctx) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("not authorized"))
	}

	previousStatus := bracket.Status

	if req.Msg.Title != nil {
		bracket.Title = *req.Msg.Title
	}
	if req.Msg.Description != nil {
		bracket.Description = *req.Msg.Description
	}
	if req.Msg.Status != nil {
		bracket.Status = *req.Msg.Status
	}
	if req.Msg.AdvanceMode != nil {
		bracket.AdvanceMode = *req.Msg.AdvanceMode
	}
	if req.Msg.RoundDurationSeconds != nil {
		bracket.RoundDurationSeconds = int(*req.Msg.RoundDurationSeconds)
	} else if req.Msg.DurationMinutes != nil {
		bracket.RoundDurationSeconds = int(*req.Msg.DurationMinutes) * 60
	}

	activate := req.Msg.Status != nil && previousStatus != "active" && bracket.Status == "active"
	advanceModeChanged := req.Msg.AdvanceMode != nil
	if bracket.AdvanceMode != "timer" {
		bracket.RoundStartedAt = nil
		bracket.RoundEndsAt = nil
	} else if bracket.Status == "active" && (activate || (advanceModeChanged && bracket.RoundEndsAt == nil)) {
		setBracketRoundWindow(&bracket, time.Now())
	}

	bracket.Prepare()

	tx, err := h.DB.Beginx()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer tx.Rollback()

	updated, err := bracket.UpdateBracket(tx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if activate || (advanceModeChanged && bracket.Status == "active") {
		if err := syncBracketMatchups(tx, &bracket); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	invalidateBracketSummaryCache(bracket.ID)

	resp := connect.NewResponse(&bracketv1.UpdateBracketResponse{
		Bracket: bracketToProto(h.DB, updated),
	})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

func (h *BracketHandler) DeleteBracket(ctx context.Context, req *connect.Request[bracketv1.DeleteBracketRequest]) (*connect.Response[bracketv1.DeleteBracketResponse], error) {
	userID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	bracketRecord, err := resolveBracketByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		} else if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var bracket models.Bracket
	if err := sqlx.GetContext(ctx, h.DB, &bracket, "SELECT * FROM brackets WHERE id = $1", bracketRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
	}

	if bracket.AuthorID != userID && !httpctx.IsAdminRequest(ctx) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("not authorized"))
	}

	s := &Server{DB: h.DB}
	if err := s.deleteBracketCascade(&bracket); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	invalidateBracketSummaryCache(bracket.ID)

	resp := connect.NewResponse(&bracketv1.DeleteBracketResponse{Message: "bracket deleted"})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

func (h *BracketHandler) AttachMatchup(ctx context.Context, req *connect.Request[bracketv1.AttachMatchupRequest]) (*connect.Response[bracketv1.AttachMatchupResponse], error) {
	userID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	bracketRecord, err := resolveBracketByIdentifier(h.DB, req.Msg.BracketId)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		} else if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var bracket models.Bracket
	if err := sqlx.GetContext(ctx, h.DB, &bracket, "SELECT * FROM brackets WHERE id = $1", bracketRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
	}

	if bracket.AuthorID != userID && !httpctx.IsAdminRequest(ctx) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("not authorized"))
	}
	if bracket.Status != "draft" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cannot modify bracket once active"))
	}

	round := int(req.Msg.Round)
	if round <= 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("round must be greater than 0"))
	}

	matchupRecord, err := resolveMatchupByIdentifier(h.DB, req.Msg.MatchupId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("matchup not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var matchup models.Matchup
	if err := sqlx.GetContext(ctx, h.DB, &matchup, "SELECT * FROM matchups WHERE id = $1", matchupRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("matchup not found"))
	}
	if matchup.AuthorID != userID && !httpctx.IsAdminRequest(ctx) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("not authorized to attach this matchup"))
	}

	matchup.BracketID = ptrUint(bracketRecord.ID)
	matchup.Round = ptrInt(round)
	if req.Msg.Seed != nil {
		s := int(*req.Msg.Seed)
		matchup.Seed = &s
	}

	// Propagate bracket tags to the matchup (only if matchup has none)
	if len(bracket.Tags) > 0 && len(matchup.Tags) == 0 {
		if _, err := h.DB.ExecContext(ctx,
			"UPDATE matchups SET bracket_id = $1, round = $2, seed = $3, tags = $4, updated_at = $5 WHERE id = $6",
			matchup.BracketID, matchup.Round, matchup.Seed, pq.StringArray(bracket.Tags), time.Now(), matchup.ID); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	} else {
		if _, err := h.DB.ExecContext(ctx,
			"UPDATE matchups SET bracket_id = $1, round = $2, seed = $3, updated_at = $4 WHERE id = $5",
			matchup.BracketID, matchup.Round, matchup.Seed, time.Now(), matchup.ID); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}
	invalidateBracketSummaryCache(bracket.ID)

	var reloaded models.Matchup
	if err := sqlx.GetContext(ctx, h.DB, &reloaded, "SELECT * FROM matchups WHERE id = $1", matchup.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := sqlx.SelectContext(ctx, h.DB, &reloaded.Items,
		"SELECT * FROM matchup_items WHERE matchup_id = $1 ORDER BY id ASC", reloaded.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := sqlx.GetContext(ctx, h.DB, &reloaded.Author,
		"SELECT * FROM users WHERE id = $1", reloaded.AuthorID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	reloaded.Author.ProcessAvatarPath()

	// reloaded.LikesCount comes from the row (migration 006).
	if content, err := models.LoadMatchupContent(h.DB, reloaded.ID); err == nil {
		reloaded.Content = content
	}

	resp := connect.NewResponse(&bracketv1.AttachMatchupResponse{
		Matchup: matchupToProto(h.DB, &reloaded, nil),
	})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

func (h *BracketHandler) DetachMatchup(ctx context.Context, req *connect.Request[bracketv1.DetachMatchupRequest]) (*connect.Response[bracketv1.DetachMatchupResponse], error) {
	userID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	matchupRecord, err := resolveMatchupByIdentifier(h.DB, req.Msg.MatchupId)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		} else if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("matchup not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var matchup models.Matchup
	if err := sqlx.GetContext(ctx, h.DB, &matchup, "SELECT * FROM matchups WHERE id = $1", matchupRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("matchup not found"))
	}

	if matchup.AuthorID != userID && !httpctx.IsAdminRequest(ctx) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("not authorized"))
	}

	previousBracketID := matchup.BracketID

	if _, err := h.DB.ExecContext(ctx,
		"UPDATE matchups SET bracket_id = NULL, round = NULL, updated_at = $1 WHERE id = $2",
		time.Now(), matchup.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if previousBracketID != nil {
		invalidateBracketSummaryCache(*previousBracketID)
	}

	resp := connect.NewResponse(&bracketv1.DetachMatchupResponse{Message: "matchup detached"})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

func (h *BracketHandler) AdvanceBracket(ctx context.Context, req *connect.Request[bracketv1.AdvanceBracketRequest]) (*connect.Response[bracketv1.AdvanceBracketResponse], error) {
	userID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	bracketRecord, err := resolveBracketByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		} else if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var bracket models.Bracket
	if err := sqlx.GetContext(ctx, h.DB, &bracket, "SELECT * FROM brackets WHERE id = $1", bracketRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
	}

	if bracket.AuthorID != userID && !httpctx.IsAdminRequest(ctx) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("not authorized"))
	}
	if bracket.Status != "active" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("bracket is not active"))
	}

	s := &Server{DB: h.DB}
	if _, err := s.advanceBracketInternal(h.DB, &bracket); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	invalidateBracketSummaryCache(bracket.ID)

	// LikesCount lives on the bracket row (migration 006).
	resp := connect.NewResponse(&bracketv1.AdvanceBracketResponse{
		Bracket: bracketToProto(h.DB, &bracket),
		Message: "bracket advanced successfully",
	})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

func (h *BracketHandler) ResolveTieAndAdvance(ctx context.Context, req *connect.Request[bracketv1.ResolveTieAndAdvanceRequest]) (*connect.Response[bracketv1.ResolveTieAndAdvanceResponse], error) {
	if req.Header().Get("X-Internal-Key") != os.Getenv("API_SECRET") {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	matchupRecord, err := resolveMatchupByIdentifier(h.DB, req.Msg.MatchupId)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		} else if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("matchup not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var matchup models.Matchup
	if err := sqlx.GetContext(ctx, h.DB, &matchup, "SELECT * FROM matchups WHERE id = $1", matchupRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("matchup not found"))
	}
	if err := sqlx.SelectContext(ctx, h.DB, &matchup.Items,
		"SELECT * FROM matchup_items WHERE matchup_id = $1 ORDER BY id ASC", matchup.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if matchup.BracketID == nil || matchup.Round == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("matchup is not part of a bracket"))
	}

	var bracket models.Bracket
	if err := sqlx.GetContext(ctx, h.DB, &bracket, "SELECT * FROM brackets WHERE id = $1", *matchup.BracketID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
	}

	if bracket.Status != "active" || *matchup.Round != bracket.CurrentRound {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("matchup is not in the current active round"))
	}

	// Resolve winner item: find by public ID
	winnerPublicID := req.Msg.WinnerItemId
	var winnerItemID uint
	for _, item := range matchup.Items {
		if item.PublicID == winnerPublicID {
			winnerItemID = item.ID
			break
		}
	}
	if winnerItemID == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid winner_item_id"))
	}

	matchup.WinnerItemID = ptrUint(winnerItemID)
	matchup.Status = matchupStatusCompleted

	if _, err := h.DB.ExecContext(ctx,
		"UPDATE matchups SET winner_item_id = $1, status = $2, updated_at = $3 WHERE id = $4",
		matchup.WinnerItemID, matchup.Status, time.Now(), matchup.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	invalidateBracketSummaryCache(*matchup.BracketID)

	s := &Server{DB: h.DB}
	if _, err := s.advanceBracketInternal(h.DB, &bracket); err != nil {
		log.Printf("advanceBracketInternal after resolve-tie: %v", err)
	}

	// LikesCount lives on the bracket row (migration 006).
	resp := connect.NewResponse(&bracketv1.ResolveTieAndAdvanceResponse{
		Bracket: bracketToProto(h.DB, &bracket),
		Message: "tie resolved and bracket advanced",
	})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

func (h *BracketHandler) InternalAdvanceBracket(ctx context.Context, req *connect.Request[bracketv1.InternalAdvanceBracketRequest]) (*connect.Response[bracketv1.InternalAdvanceBracketResponse], error) {
	if req.Header().Get("X-Internal-Key") != os.Getenv("API_SECRET") {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	bracketRecord, err := resolveBracketByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		} else if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var bracket models.Bracket
	if err := sqlx.GetContext(ctx, h.DB, &bracket, "SELECT * FROM brackets WHERE id = $1", bracketRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
	}

	if bracket.Status != "active" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("bracket is not active"))
	}

	s := &Server{DB: h.DB}
	if _, err := s.advanceBracketInternal(h.DB, &bracket); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	invalidateBracketSummaryCache(bracket.ID)

	resp := connect.NewResponse(&bracketv1.InternalAdvanceBracketResponse{
		Message: "bracket advanced successfully",
	})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}
