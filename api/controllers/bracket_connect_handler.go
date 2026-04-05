package controllers

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"time"

	bracketv1 "Matchup/gen/bracket/v1"
	"Matchup/gen/bracket/v1/bracketv1connect"
	matchupv1 "Matchup/gen/matchup/v1"
	"Matchup/models"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

var _ bracketv1connect.BracketServiceHandler = (*BracketHandler)(nil)

// BracketHandler implements bracketv1connect.BracketServiceHandler.
type BracketHandler struct {
	DB *sqlx.DB
}

func (h *BracketHandler) GetPopularBrackets(ctx context.Context, req *connect.Request[bracketv1.GetPopularBracketsRequest]) (*connect.Response[bracketv1.GetPopularBracketsResponse], error) {
	const limit = 5
	viewerID, hasViewer := optionalViewerFromCtx(ctx)
	isAdmin := httpctx.IsAdminRequest(ctx)

	var rows []popularBracketRow
	if err := sqlx.SelectContext(ctx, h.DB, &rows,
		"SELECT * FROM popular_brackets_snapshot ORDER BY rank ASC LIMIT $1", limit); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	bracketIDs := make([]uint, 0, len(rows))
	authorIDs := make([]uint, 0, len(rows))
	for _, row := range rows {
		bracketIDs = append(bracketIDs, row.ID)
		authorIDs = append(authorIDs, row.AuthorID)
	}

	bracketPublicIDs := loadBracketPublicIDMap(h.DB, bracketIDs)
	authorPublicIDs := loadUserPublicIDMap(h.DB, authorIDs)

	var protoBrackets []*bracketv1.PopularBracketData
	for i := range rows {
		var bracket models.Bracket
		if err := sqlx.GetContext(ctx, h.DB, &bracket, "SELECT * FROM brackets WHERE id = $1", rows[i].ID); err != nil {
			continue
		}
		if err := sqlx.GetContext(ctx, h.DB, &bracket.Author, "SELECT * FROM users WHERE id = $1", bracket.AuthorID); err != nil {
			continue
		}
		bracket.Author.ProcessAvatarPath()

		allowed, _, err := canViewUserContent(h.DB, viewerID, hasViewer, &bracket.Author, bracket.Visibility, isAdmin)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		if !allowed || bracket.Status != "active" {
			continue
		}

		var likesCount int64
		_ = sqlx.GetContext(ctx, h.DB, &likesCount, "SELECT COUNT(*) FROM bracket_likes WHERE bracket_id = $1", bracket.ID)
		var commentsCount int64
		_ = sqlx.GetContext(ctx, h.DB, &commentsCount, "SELECT COUNT(*) FROM bracket_comments WHERE bracket_id = $1", bracket.ID)

		dto := PopularBracketDTO{
			ID:              bracketPublicIDs[rows[i].ID],
			Title:           rows[i].Title,
			AuthorID:        authorPublicIDs[rows[i].AuthorID],
			CurrentRound:    rows[i].CurrentRound,
			Size:            bracket.Size,
			Votes:           0,
			Likes:           likesCount,
			Comments:        commentsCount,
			EngagementScore: rows[i].EngagementScore,
			Rank:            rows[i].Rank,
		}
		protoBrackets = append(protoBrackets, popularBracketToProto(dto))
	}

	return connect.NewResponse(&bracketv1.GetPopularBracketsResponse{
		Brackets: protoBrackets,
	}), nil
}

func (h *BracketHandler) GetBracket(ctx context.Context, req *connect.Request[bracketv1.GetBracketRequest]) (*connect.Response[bracketv1.GetBracketResponse], error) {
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

	if found.Status == "active" &&
		found.AdvanceMode == "timer" &&
		found.RoundEndsAt != nil &&
		!time.Now().Before(*found.RoundEndsAt) {
		s := &Server{DB: h.DB}
		if _, err := s.advanceBracketInternal(h.DB, found); err != nil {
			log.Printf("auto advance bracket %d: %v", found.ID, err)
		} else if refreshed, err := bracket.FindBracketByID(h.DB, found.ID); err == nil {
			found = refreshed
		}
	}

	var likesCount int64
	_ = sqlx.GetContext(ctx, h.DB, &likesCount, "SELECT COUNT(*) FROM bracket_likes WHERE bracket_id = $1", found.ID)
	found.LikesCount = likesCount

	return connect.NewResponse(&bracketv1.GetBracketResponse{
		Bracket: bracketToProto(h.DB, found),
	}), nil
}

func (h *BracketHandler) GetBracketSummary(ctx context.Context, req *connect.Request[bracketv1.GetBracketSummaryRequest]) (*connect.Response[bracketv1.GetBracketSummaryResponse], error) {
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

	if bracket.Status == "active" &&
		bracket.AdvanceMode == "timer" &&
		bracket.RoundEndsAt != nil &&
		!time.Now().Before(*bracket.RoundEndsAt) {
		s := &Server{DB: h.DB}
		if _, err := s.advanceBracketInternal(h.DB, &bracket); err != nil {
			log.Printf("auto advance bracket %d: %v", bracket.ID, err)
		}
	}

	matchups, err := models.FindMatchupsByBracket(h.DB, bracketRecord.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	for i := range matchups {
		if err := dedupeBracketMatchupItems(h.DB, &matchups[i]); err != nil {
			log.Printf("dedupe bracket matchup %d: %v", matchups[i].ID, err)
		}
	}

	likedMatchupIDs := []uint{}
	likedBracket := false
	var bracketLikesCount int64

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
	_ = sqlx.GetContext(ctx, h.DB, &bracketLikesCount, "SELECT COUNT(*) FROM bracket_likes WHERE bracket_id = $1", bracketRecord.ID)
	bracket.LikesCount = bracketLikesCount

	var protoMatchups []*matchupv1.MatchupData
	matchupPublicIDs := make(map[uint]string, len(matchups))
	for i := range matchups {
		var likesCount int64
		_ = sqlx.GetContext(ctx, h.DB, &likesCount, "SELECT COUNT(*) FROM likes WHERE matchup_id = $1", matchups[i].ID)
		protoMatchups = append(protoMatchups, matchupToProto(h.DB, &matchups[i], nil, likesCount))
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
	owner, err := resolveUserByIdentifier(h.DB, req.Msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	}

	viewerID, hasViewer := optionalViewerFromCtx(ctx)
	isAdmin := httpctx.IsAdminRequest(ctx)
	allowed, reason, err := canViewUserContent(h.DB, viewerID, hasViewer, owner, visibilityPublic, isAdmin)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New(visibilityErrorMessage(reason)))
	}

	var bracket models.Bracket
	brackets, err := bracket.FindUserBrackets(h.DB, owner.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var protoBrackets []*bracketv1.BracketData
	for i := range *brackets {
		b := &(*brackets)[i]
		allowed, _, err := canViewUserContent(h.DB, viewerID, hasViewer, owner, b.Visibility, isAdmin)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		if !allowed {
			continue
		}
		var likesCount int64
		_ = sqlx.GetContext(ctx, h.DB, &likesCount, "SELECT COUNT(*) FROM bracket_likes WHERE bracket_id = $1", b.ID)
		b.LikesCount = likesCount
		protoBrackets = append(protoBrackets, bracketToProto(h.DB, b))
	}

	return connect.NewResponse(&bracketv1.GetUserBracketsResponse{
		Brackets: protoBrackets,
	}), nil
}

func (h *BracketHandler) GetBracketMatchups(ctx context.Context, req *connect.Request[bracketv1.GetBracketMatchupsRequest]) (*connect.Response[bracketv1.GetBracketMatchupsResponse], error) {
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

	if found.Status == "active" &&
		found.AdvanceMode == "timer" &&
		found.RoundEndsAt != nil &&
		!time.Now().Before(*found.RoundEndsAt) {
		s := &Server{DB: h.DB}
		if _, err := s.advanceBracketInternal(h.DB, found); err != nil {
			log.Printf("auto advance bracket %d: %v", found.ID, err)
		}
	}

	matchups, err := models.FindMatchupsByBracket(h.DB, bracketRecord.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	for i := range matchups {
		if err := dedupeBracketMatchupItems(h.DB, &matchups[i]); err != nil {
			log.Printf("dedupe bracket matchup %d: %v", matchups[i].ID, err)
		}
	}

	protoMatchups := make([]*matchupv1.MatchupData, 0, len(matchups))
	for i := range matchups {
		var likesCount int64
		_ = sqlx.GetContext(ctx, h.DB, &likesCount, "SELECT COUNT(*) FROM likes WHERE matchup_id = $1", matchups[i].ID)
		protoMatchups = append(protoMatchups, matchupToProto(h.DB, &matchups[i], nil, likesCount))
	}

	return connect.NewResponse(&bracketv1.GetBracketMatchupsResponse{
		Matchups: protoMatchups,
	}), nil
}

func (h *BracketHandler) CreateBracket(ctx context.Context, req *connect.Request[bracketv1.CreateBracketRequest]) (*connect.Response[bracketv1.CreateBracketResponse], error) {
	userID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
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
	newBracket, err := bracket.SaveBracket(h.DB)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	generateFullBracket(h.DB, *newBracket, req.Msg.Entries)

	return connect.NewResponse(&bracketv1.CreateBracketResponse{
		Bracket: bracketToProto(h.DB, newBracket),
	}), nil
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

	return connect.NewResponse(&bracketv1.UpdateBracketResponse{
		Bracket: bracketToProto(h.DB, updated),
	}), nil
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

	return connect.NewResponse(&bracketv1.DeleteBracketResponse{Message: "bracket deleted"}), nil
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

	var likesCount int64
	_ = sqlx.GetContext(ctx, h.DB, &likesCount, "SELECT COUNT(*) FROM likes WHERE matchup_id = $1", reloaded.ID)

	return connect.NewResponse(&bracketv1.AttachMatchupResponse{
		Matchup: matchupToProto(h.DB, &reloaded, nil, likesCount),
	}), nil
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

	return connect.NewResponse(&bracketv1.DetachMatchupResponse{Message: "matchup detached"}), nil
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

	var likesCount int64
	_ = sqlx.GetContext(ctx, h.DB, &likesCount, "SELECT COUNT(*) FROM bracket_likes WHERE bracket_id = $1", bracket.ID)
	bracket.LikesCount = likesCount

	return connect.NewResponse(&bracketv1.AdvanceBracketResponse{
		Bracket: bracketToProto(h.DB, &bracket),
		Message: "bracket advanced successfully",
	}), nil
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

	var likesCount int64
	_ = sqlx.GetContext(ctx, h.DB, &likesCount, "SELECT COUNT(*) FROM bracket_likes WHERE bracket_id = $1", bracket.ID)
	bracket.LikesCount = likesCount

	return connect.NewResponse(&bracketv1.ResolveTieAndAdvanceResponse{
		Bracket: bracketToProto(h.DB, &bracket),
		Message: "tie resolved and bracket advanced",
	}), nil
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

	return connect.NewResponse(&bracketv1.InternalAdvanceBracketResponse{
		Message: "bracket advanced successfully",
	}), nil
}
