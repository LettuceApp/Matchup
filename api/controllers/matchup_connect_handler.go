package controllers

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"Matchup/cache"
	matchupv1 "Matchup/gen/matchup/v1"
	"Matchup/gen/matchup/v1/matchupv1connect"
	"Matchup/models"
	"Matchup/utils/fileformat"
	httpctx "Matchup/utils/httpctx"

	aws2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/lib/pq"
	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
)

var _ matchupv1connect.MatchupServiceHandler = (*MatchupHandler)(nil)

// MatchupHandler implements matchupv1connect.MatchupServiceHandler.
type MatchupHandler struct {
	DB *sqlx.DB
}

func (h *MatchupHandler) ListMatchups(ctx context.Context, req *connect.Request[matchupv1.ListMatchupsRequest]) (*connect.Response[matchupv1.ListMatchupsResponse], error) {
	page := int(req.Msg.GetPage())
	if page < 1 {
		page = 1
	}
	limit := int(req.Msg.GetLimit())
	if limit < 1 || limit > 100 {
		limit = 10
	}

	viewerID, hasViewer := optionalViewerFromCtx(ctx)
	isAdmin := httpctx.IsAdminRequest(ctx)
	offset := (page - 1) * limit

	var total int64
	if err := sqlx.GetContext(ctx, h.DB, &total, "SELECT COUNT(*) FROM matchups"); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var matchups []models.Matchup
	if err := sqlx.SelectContext(ctx, h.DB, &matchups,
		"SELECT * FROM matchups ORDER BY created_at DESC LIMIT $1 OFFSET $2", limit, offset); err != nil && !errors.Is(err, sql.ErrNoRows) {
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
		itemQuery = h.DB.Rebind(itemQuery)
		var allItems []models.MatchupItem
		_ = sqlx.SelectContext(ctx, h.DB, &allItems, itemQuery, itemArgs...)
		itemsByMatchup := make(map[uint][]models.MatchupItem)
		for _, item := range allItems {
			itemsByMatchup[item.MatchupID] = append(itemsByMatchup[item.MatchupID], item)
		}

		authorQuery, authorArgs, _ := sqlx.In("SELECT * FROM users WHERE id IN (?)", authorIDs)
		authorQuery = h.DB.Rebind(authorQuery)
		var authors []models.User
		_ = sqlx.SelectContext(ctx, h.DB, &authors, authorQuery, authorArgs...)
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

	var protoMatchups []*matchupv1.MatchupData
	for i := range matchups {
		m := &matchups[i]
		allowed, _, err := canViewUserContent(h.DB, viewerID, hasViewer, &m.Author, m.Visibility, isAdmin)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		if !allowed {
			continue
		}
		if err := finalizeStandaloneMatchupIfExpiredStandalone(h.DB, m); err != nil {
			log.Printf("auto finalize matchup %d: %v", m.ID, err)
		}

		var likesCount int64
		_ = sqlx.GetContext(ctx, h.DB, &likesCount, "SELECT COUNT(*) FROM likes WHERE matchup_id = $1", m.ID)

		comment := models.Comment{}
		comments, err := comment.GetComments(h.DB, m.ID)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}

		protoMatchups = append(protoMatchups, matchupToProto(h.DB, m, *comments, likesCount))
	}

	return connect.NewResponse(&matchupv1.ListMatchupsResponse{
		Matchups:   protoMatchups,
		Pagination: paginationToProto(page, limit, total),
	}), nil
}

func (h *MatchupHandler) GetMatchup(ctx context.Context, req *connect.Request[matchupv1.GetMatchupRequest]) (*connect.Response[matchupv1.GetMatchupResponse], error) {
	matchupRecord, err := resolveMatchupByIdentifier(h.DB, req.Msg.Id)
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
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("matchup not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := sqlx.SelectContext(ctx, h.DB, &matchup.Items,
		"SELECT * FROM matchup_items WHERE matchup_id = $1 ORDER BY id ASC", matchup.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := sqlx.GetContext(ctx, h.DB, &matchup.Author,
		"SELECT * FROM users WHERE id = $1", matchup.AuthorID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	matchup.Author.ProcessAvatarPath()

	viewerID, hasViewer := optionalViewerFromCtx(ctx)
	allowed, reason, err := canViewUserContent(h.DB, viewerID, hasViewer, &matchup.Author, matchup.Visibility, httpctx.IsAdminRequest(ctx))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New(visibilityErrorMessage(reason)))
	}

	if err := dedupeBracketMatchupItems(h.DB, &matchup); err != nil {
		log.Printf("dedupe bracket matchup %d: %v", matchup.ID, err)
	}
	if err := finalizeStandaloneMatchupIfExpiredStandalone(h.DB, &matchup); err != nil {
		log.Printf("auto finalize matchup %d: %v", matchup.ID, err)
	}

	var likesCount int64
	_ = sqlx.GetContext(ctx, h.DB, &likesCount, "SELECT COUNT(*) FROM likes WHERE matchup_id = $1", matchup.ID)

	comment := models.Comment{}
	comments, err := comment.GetComments(h.DB, matchup.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&matchupv1.GetMatchupResponse{
		Matchup: matchupToProto(h.DB, &matchup, *comments, likesCount),
	}), nil
}

func (h *MatchupHandler) GetPopularMatchups(ctx context.Context, req *connect.Request[matchupv1.GetPopularMatchupsRequest]) (*connect.Response[matchupv1.GetPopularMatchupsResponse], error) {
	const limit = 5
	viewerID, hasViewer := optionalViewerFromCtx(ctx)
	isAdmin := httpctx.IsAdminRequest(ctx)

	var rows []popularMatchupRow
	if err := sqlx.SelectContext(ctx, h.DB, &rows,
		"SELECT * FROM popular_matchups_snapshot ORDER BY rank ASC LIMIT $1", limit); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	matchupIDs := make([]uint, 0, len(rows))
	authorIDs := make([]uint, 0, len(rows))
	bracketIDs := make([]uint, 0, len(rows))
	bracketAuthorIDs := make([]uint, 0, len(rows))
	for _, row := range rows {
		matchupIDs = append(matchupIDs, row.ID)
		authorIDs = append(authorIDs, row.AuthorID)
		if row.BracketID != nil {
			bracketIDs = append(bracketIDs, *row.BracketID)
		}
		if row.BracketAuthorID != nil {
			bracketAuthorIDs = append(bracketAuthorIDs, *row.BracketAuthorID)
		}
	}

	matchupPublicIDs := loadMatchupPublicIDMap(h.DB, matchupIDs)
	authorPublicIDs := loadUserPublicIDMap(h.DB, authorIDs)
	bracketPublicIDs := loadBracketPublicIDMap(h.DB, bracketIDs)
	bracketAuthorPublicIDs := loadUserPublicIDMap(h.DB, bracketAuthorIDs)

	var protoMatchups []*matchupv1.PopularMatchupData
	for i := range rows {
		var matchup models.Matchup
		if err := sqlx.GetContext(ctx, h.DB, &matchup, "SELECT * FROM matchups WHERE id = $1", rows[i].ID); err != nil {
			continue
		}
		if err := sqlx.GetContext(ctx, h.DB, &matchup.Author, "SELECT * FROM users WHERE id = $1", matchup.AuthorID); err != nil {
			continue
		}
		matchup.Author.ProcessAvatarPath()

		allowed, _, err := canViewUserContent(h.DB, viewerID, hasViewer, &matchup.Author, matchup.Visibility, isAdmin)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		if !allowed {
			continue
		}

		var bracketID *string
		if rows[i].BracketID != nil {
			if id := bracketPublicIDs[*rows[i].BracketID]; id != "" {
				bracketID = &id
			}
		}
		var bracketAuthorID *string
		if rows[i].BracketAuthorID != nil {
			if id := bracketAuthorPublicIDs[*rows[i].BracketAuthorID]; id != "" {
				bracketAuthorID = &id
			}
		}

		dto := PopularMatchupDTO{
			ID:              matchupPublicIDs[rows[i].ID],
			Title:           rows[i].Title,
			AuthorID:        authorPublicIDs[rows[i].AuthorID],
			BracketID:       bracketID,
			BracketAuthorID: bracketAuthorID,
			Round:           rows[i].Round,
			CurrentRound:    rows[i].CurrentRound,
			Votes:           rows[i].Votes,
			Likes:           rows[i].Likes,
			Comments:        rows[i].Comments,
			EngagementScore: rows[i].EngagementScore,
			Rank:            rows[i].Rank,
		}
		protoMatchups = append(protoMatchups, popularMatchupToProto(dto))
	}

	return connect.NewResponse(&matchupv1.GetPopularMatchupsResponse{
		Matchups: protoMatchups,
	}), nil
}

func (h *MatchupHandler) GetUserMatchups(ctx context.Context, req *connect.Request[matchupv1.GetUserMatchupsRequest]) (*connect.Response[matchupv1.GetUserMatchupsResponse], error) {
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
	if err := sqlx.GetContext(ctx, h.DB, &total,
		"SELECT COUNT(*) FROM matchups WHERE author_id = $1", owner.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var matchups []models.Matchup
	if err := sqlx.SelectContext(ctx, h.DB, &matchups,
		"SELECT * FROM matchups WHERE author_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3",
		owner.ID, limit, offset); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if len(matchups) > 0 {
		matchupIDs := make([]uint, len(matchups))
		for i := range matchups {
			matchupIDs[i] = matchups[i].ID
			matchups[i].Author = *owner
		}

		itemQuery, itemArgs, _ := sqlx.In("SELECT * FROM matchup_items WHERE matchup_id IN (?)", matchupIDs)
		itemQuery = h.DB.Rebind(itemQuery)
		var allItems []models.MatchupItem
		_ = sqlx.SelectContext(ctx, h.DB, &allItems, itemQuery, itemArgs...)
		itemsByMatchup := make(map[uint][]models.MatchupItem)
		for _, item := range allItems {
			itemsByMatchup[item.MatchupID] = append(itemsByMatchup[item.MatchupID], item)
		}
		for i := range matchups {
			matchups[i].Items = itemsByMatchup[matchups[i].ID]
		}
	}

	var protoMatchups []*matchupv1.MatchupData
	for i := range matchups {
		m := &matchups[i]
		allowed, _, err := canViewUserContent(h.DB, viewerID, hasViewer, owner, m.Visibility, isAdmin)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		if !allowed {
			continue
		}
		if err := finalizeStandaloneMatchupIfExpiredStandalone(h.DB, m); err != nil {
			log.Printf("auto finalize matchup %d: %v", m.ID, err)
		}

		comment := models.Comment{}
		comments, err := comment.GetComments(h.DB, m.ID)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}

		var likesCount int64
		_ = sqlx.GetContext(ctx, h.DB, &likesCount, "SELECT COUNT(*) FROM likes WHERE matchup_id = $1", m.ID)

		protoMatchups = append(protoMatchups, matchupToProto(h.DB, m, *comments, likesCount))
	}

	return connect.NewResponse(&matchupv1.GetUserMatchupsResponse{
		Matchups:   protoMatchups,
		Pagination: paginationToProto(page, limit, total),
	}), nil
}

func (h *MatchupHandler) CreateMatchup(ctx context.Context, req *connect.Request[matchupv1.CreateMatchupRequest]) (*connect.Response[matchupv1.CreateMatchupResponse], error) {
	userID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	matchup := models.Matchup{}
	matchup.AuthorID = userID
	matchup.Title = req.Msg.Title
	if req.Msg.Content != nil {
		matchup.Content = *req.Msg.Content
	}
	if req.Msg.EndMode != nil {
		matchup.EndMode = *req.Msg.EndMode
	}
	if req.Msg.DurationSeconds != nil {
		matchup.DurationSeconds = int(*req.Msg.DurationSeconds)
	}

	// Resolve bracket ID from public ID if provided
	if req.Msg.BracketId != nil {
		bracketRecord, err := resolveBracketByIdentifier(h.DB, *req.Msg.BracketId)
		if err != nil {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
		}
		matchup.BracketID = &bracketRecord.ID
	}

	if req.Msg.Round != nil {
		r := int(*req.Msg.Round)
		matchup.Round = &r
	}
	if req.Msg.Seed != nil {
		s := int(*req.Msg.Seed)
		matchup.Seed = &s
	}

	for _, it := range req.Msg.Items {
		matchup.Items = append(matchup.Items, models.MatchupItem{Item: it.Item})
	}

	// Tags
	if len(req.Msg.Tags) > 0 {
		matchup.Tags = pq.StringArray(req.Msg.Tags)
	} else {
		matchup.Tags = pq.StringArray{}
	}

	// If a bracket is attached, inherit its tags
	if matchup.BracketID != nil && len(matchup.Tags) == 0 {
		var bracketTags pq.StringArray
		_ = h.DB.QueryRowContext(context.Background(),
			"SELECT tags FROM brackets WHERE id = $1", *matchup.BracketID).Scan(&bracketTags)
		if len(bracketTags) > 0 {
			matchup.Tags = bracketTags
		}
	}

	// Optional image upload to S3
	if len(req.Msg.ImageData) > 0 {
		buf := req.Msg.ImageData
		if len(buf) > 5_000_000 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("image too large (max 5MB)"))
		}
		fileType := http.DetectContentType(buf)
		if !strings.HasPrefix(fileType, "image/") {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("not an image"))
		}
		var ext string
		switch fileType {
		case "image/png":
			ext = ".png"
		case "image/gif":
			ext = ".gif"
		case "image/webp":
			ext = ".webp"
		default:
			ext = ".jpg"
		}
		filePath := fileformat.UniqueFormat("matchup" + ext)
		key := "MatchupImages/" + filePath

		rawBucket := os.Getenv("S3_BUCKET")
		bucketName := strings.SplitN(rawBucket, "/", 2)[0]
		region := os.Getenv("AWS_REGION")
		if region == "" {
			region = "us-east-2"
		}
		var s3cfg aws2.Config
		accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
		secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
		sessionToken := os.Getenv("AWS_SESSION_TOKEN")
		var cfgErr error
		if accessKey != "" && secretKey != "" {
			s3cfg, cfgErr = config.LoadDefaultConfig(context.TODO(),
				config.WithRegion(region),
				config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, sessionToken)),
			)
		} else {
			s3cfg, cfgErr = config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
		}
		if cfgErr != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("AWS configuration error: %w", cfgErr))
		}
		s3Client := s3.NewFromConfig(s3cfg)
		size := int64(len(buf))
		if _, uploadErr := s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
			Bucket:        aws2.String(bucketName),
			Key:           aws2.String(key),
			Body:          bytes.NewReader(buf),
			ContentLength: aws2.Int64(size),
			ContentType:   aws2.String(fileType),
		}); uploadErr != nil {
			log.Printf("S3 upload failed: %v", uploadErr)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to upload image"))
		}
		matchup.ImagePath = filePath
	}

	if matchup.BracketID == nil && len(matchup.Items) > 4 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("standalone matchups can only have up to 4 items"))
	}
	if matchup.BracketID != nil && len(matchup.Items) > 2 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("bracket matchups can only have 2 items"))
	}

	if matchup.BracketID == nil && matchup.Status == "" {
		matchup.Status = matchupStatusActive
	}
	matchup.Prepare()
	if matchup.BracketID == nil {
		if matchup.EndMode == "timer" {
			matchup.DurationSeconds = normalizeMatchupDurationSeconds(matchup.DurationSeconds)
		} else {
			matchup.DurationSeconds = 0
		}
	}
	if matchup.BracketID == nil && isMatchupOpenStatus(matchup.Status) {
		now := time.Now()
		if matchup.StartTime == nil {
			matchup.StartTime = &now
		}
		if matchup.EndMode == "timer" && matchup.EndTime == nil {
			end := matchup.StartTime.Add(time.Duration(matchup.DurationSeconds) * time.Second)
			matchup.EndTime = &end
		}
	}
	if errs := matchup.Validate(); len(errs) > 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("validation failed: %v", errs))
	}

	tx, err := h.DB.Beginx()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer tx.Rollback()

	matchupCreated, err := matchup.SaveMatchup(tx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if err := sqlx.SelectContext(ctx, h.DB, &matchupCreated.Items,
		"SELECT * FROM matchup_items WHERE matchup_id = $1 ORDER BY id ASC", matchupCreated.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := sqlx.GetContext(ctx, h.DB, &matchupCreated.Author,
		"SELECT * FROM users WHERE id = $1", matchupCreated.AuthorID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	matchupCreated.Author.ProcessAvatarPath()

	comment := models.Comment{}
	comments, err := comment.GetComments(h.DB, matchupCreated.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var likesCount int64
	_ = sqlx.GetContext(ctx, h.DB, &likesCount, "SELECT COUNT(*) FROM likes WHERE matchup_id = $1", matchupCreated.ID)

	return connect.NewResponse(&matchupv1.CreateMatchupResponse{
		Matchup: matchupToProto(h.DB, matchupCreated, *comments, likesCount),
	}), nil
}

func (h *MatchupHandler) UpdateMatchup(ctx context.Context, req *connect.Request[matchupv1.UpdateMatchupRequest]) (*connect.Response[matchupv1.UpdateMatchupResponse], error) {
	requestorID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	matchupRecord, err := resolveMatchupByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		} else if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("matchup not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var existing models.Matchup
	if err := sqlx.GetContext(ctx, h.DB, &existing, "SELECT * FROM matchups WHERE id = $1", matchupRecord.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("matchup not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if existing.AuthorID != requestorID && !httpctx.IsAdminRequest(ctx) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("not authorized to update this matchup"))
	}

	if req.Msg.Title != nil {
		existing.Title = *req.Msg.Title
	}
	if req.Msg.Content != nil {
		existing.Content = *req.Msg.Content
	}
	existing.UpdatedAt = time.Now()
	existing.Prepare()
	if errs := existing.Validate(); len(errs) > 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("validation failed: %v", errs))
	}

	if _, err := h.DB.ExecContext(ctx,
		"UPDATE matchups SET title = $1, content = $2, updated_at = $3 WHERE id = $4",
		existing.Title, existing.Content, existing.UpdatedAt, existing.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var updated models.Matchup
	if err := sqlx.GetContext(ctx, h.DB, &updated, "SELECT * FROM matchups WHERE id = $1", existing.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := sqlx.SelectContext(ctx, h.DB, &updated.Items,
		"SELECT * FROM matchup_items WHERE matchup_id = $1 ORDER BY id ASC", updated.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := sqlx.GetContext(ctx, h.DB, &updated.Author,
		"SELECT * FROM users WHERE id = $1", updated.AuthorID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	updated.Author.ProcessAvatarPath()

	comment := models.Comment{}
	comments, err := comment.GetComments(h.DB, updated.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	var likesCount int64
	_ = sqlx.GetContext(ctx, h.DB, &likesCount, "SELECT COUNT(*) FROM likes WHERE matchup_id = $1", updated.ID)

	return connect.NewResponse(&matchupv1.UpdateMatchupResponse{
		Matchup: matchupToProto(h.DB, &updated, *comments, likesCount),
	}), nil
}

func (h *MatchupHandler) DeleteMatchup(ctx context.Context, req *connect.Request[matchupv1.DeleteMatchupRequest]) (*connect.Response[matchupv1.DeleteMatchupResponse], error) {
	requestorID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	matchupRecord, err := resolveMatchupByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		} else if errors.Is(err, sql.ErrNoRows) {
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
	if m.AuthorID != requestorID && !httpctx.IsAdminRequest(ctx) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("not authorized to delete this matchup"))
	}

	if err := deleteMatchupCascadeStandalone(h.DB, &m); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&matchupv1.DeleteMatchupResponse{Message: "matchup deleted"}), nil
}

func (h *MatchupHandler) OverrideMatchupWinner(ctx context.Context, req *connect.Request[matchupv1.OverrideMatchupWinnerRequest]) (*connect.Response[matchupv1.OverrideMatchupWinnerResponse], error) {
	userID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	matchupRecord, err := resolveMatchupByIdentifier(h.DB, req.Msg.Id)
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

	if matchup.AuthorID != userID && !httpctx.IsAdminRequest(ctx) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("not authorized"))
	}
	if matchup.Status == matchupStatusCompleted && matchup.BracketID == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("matchup is completed and cannot be overridden"))
	}
	if matchup.BracketID != nil {
		var b models.Bracket
		if err := sqlx.GetContext(ctx, h.DB, &b, "SELECT * FROM brackets WHERE id = $1", *matchup.BracketID); err != nil {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
		}
		if b.Status != "active" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cannot override winner for this bracket at this time"))
		}
		if matchup.Round == nil || *matchup.Round != b.CurrentRound {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cannot override winner outside the current round"))
		}
	}

	// Resolve winner item public ID to internal ID
	winnerPublicID := req.Msg.WinnerItemId
	var winnerItemID uint
	winnerPublicIDOut := ""
	for _, item := range matchup.Items {
		if item.PublicID == winnerPublicID {
			winnerItemID = item.ID
			winnerPublicIDOut = item.PublicID
			break
		}
	}
	if winnerItemID == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid winner item"))
	}

	matchup.WinnerItemID = &winnerItemID
	matchup.Status = matchupStatusCompleted

	if _, err := h.DB.ExecContext(ctx,
		"UPDATE matchups SET winner_item_id = $1, status = $2, updated_at = $3 WHERE id = $4",
		matchup.WinnerItemID, matchup.Status, time.Now(), matchup.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if matchup.BracketID != nil {
		invalidateBracketSummaryCache(*matchup.BracketID)
	}

	return connect.NewResponse(&matchupv1.OverrideMatchupWinnerResponse{
		Message:      "winner selected",
		WinnerItemId: winnerPublicIDOut,
	}), nil
}

func (h *MatchupHandler) CompleteMatchup(ctx context.Context, req *connect.Request[matchupv1.CompleteMatchupRequest]) (*connect.Response[matchupv1.CompleteMatchupResponse], error) {
	userID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	matchupRecord, err := resolveMatchupByIdentifier(h.DB, req.Msg.Id)
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

	if matchup.AuthorID != userID && !httpctx.IsAdminRequest(ctx) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("not authorized"))
	}

	if matchup.BracketID != nil {
		var bracket models.Bracket
		if err := sqlx.GetContext(ctx, h.DB, &bracket,
			"SELECT * FROM brackets WHERE id = $1", *matchup.BracketID); err != nil {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("bracket not found"))
		}
		if bracket.Status != "active" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cannot modify matchup for an inactive bracket"))
		}
		if matchup.Round == nil || *matchup.Round != bracket.CurrentRound {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cannot modify matchup outside the current bracket round"))
		}
	}

	// Toggle off if already completed
	if matchup.Status == matchupStatusCompleted {
		if _, err := h.DB.ExecContext(ctx,
			"UPDATE matchups SET status = $1, winner_item_id = NULL, updated_at = $2 WHERE id = $3",
			matchupStatusActive, time.Now(), matchup.ID); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		if matchup.BracketID != nil {
			invalidateBracketSummaryCache(*matchup.BracketID)
		}
		return connect.NewResponse(&matchupv1.CompleteMatchupResponse{
			Message: "matchup reopened",
			Status:  matchupStatusActive,
		}), nil
	}

	winnerID, err := determineMatchupWinner(&matchup)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	matchup.Status = matchupStatusCompleted
	matchup.WinnerItemID = &winnerID

	if _, err := h.DB.ExecContext(ctx,
		"UPDATE matchups SET status = $1, winner_item_id = $2, updated_at = $3 WHERE id = $4",
		matchup.Status, matchup.WinnerItemID, time.Now(), matchup.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if matchup.BracketID != nil {
		invalidateBracketSummaryCache(*matchup.BracketID)
	}

	winnerPublicID := ""
	for _, item := range matchup.Items {
		if item.ID == winnerID {
			winnerPublicID = item.PublicID
			break
		}
	}
	if winnerPublicID == "" {
		winnerPublicID = resolveMatchupItemPublicID(h.DB, winnerID)
	}

	return connect.NewResponse(&matchupv1.CompleteMatchupResponse{
		Message:      "matchup completed",
		Status:       matchup.Status,
		WinnerItemId: winnerPublicID,
	}), nil
}

func (h *MatchupHandler) ReadyUpMatchup(ctx context.Context, req *connect.Request[matchupv1.ReadyUpMatchupRequest]) (*connect.Response[matchupv1.ReadyUpMatchupResponse], error) {
	userID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	matchupRecord, err := resolveMatchupByIdentifier(h.DB, req.Msg.Id)
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
	if err := sqlx.GetContext(ctx, h.DB, &matchup.Author,
		"SELECT * FROM users WHERE id = $1", matchup.AuthorID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	matchup.Author.ProcessAvatarPath()

	if matchup.AuthorID != userID && !httpctx.IsAdminRequest(ctx) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("not authorized"))
	}
	if matchup.WinnerItemID == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("select a winner first"))
	}

	if _, err := h.DB.ExecContext(ctx,
		"UPDATE matchups SET status = $1, updated_at = $2 WHERE id = $3",
		matchupStatusCompleted, time.Now(), matchup.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if matchup.BracketID != nil {
		invalidateBracketSummaryCache(*matchup.BracketID)
	}

	matchup.Status = matchupStatusCompleted
	var likesCount int64
	_ = sqlx.GetContext(ctx, h.DB, &likesCount, "SELECT COUNT(*) FROM likes WHERE matchup_id = $1", matchup.ID)

	return connect.NewResponse(&matchupv1.ReadyUpMatchupResponse{
		Matchup: matchupToProto(h.DB, &matchup, nil, likesCount),
	}), nil
}

func (h *MatchupHandler) ActivateMatchup(ctx context.Context, req *connect.Request[matchupv1.ActivateMatchupRequest]) (*connect.Response[matchupv1.ActivateMatchupResponse], error) {
	userID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	matchupRecord, err := resolveMatchupByIdentifier(h.DB, req.Msg.Id)
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
	if matchup.BracketID != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("bracket matchups cannot be manually activated"))
	}

	now := time.Now()
	if matchup.StartTime == nil {
		matchup.StartTime = &now
	}
	matchup.Status = matchupStatusActive

	if matchup.EndMode == "manual" {
		matchup.EndTime = nil
	} else if matchup.EndTime == nil {
		durationSeconds := normalizeMatchupDurationSeconds(matchup.DurationSeconds)
		matchup.DurationSeconds = durationSeconds
		end := matchup.StartTime.Add(time.Duration(durationSeconds) * time.Second)
		matchup.EndTime = &end
	}

	if _, err := h.DB.ExecContext(ctx,
		`UPDATE matchups SET status = $1, start_time = $2, end_time = $3,
		 duration_seconds = $4, updated_at = $5 WHERE id = $6`,
		matchup.Status, matchup.StartTime, matchup.EndTime,
		matchup.DurationSeconds, time.Now(), matchup.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var updated models.Matchup
	if err := sqlx.GetContext(ctx, h.DB, &updated, "SELECT * FROM matchups WHERE id = $1", matchup.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := sqlx.SelectContext(ctx, h.DB, &updated.Items,
		"SELECT * FROM matchup_items WHERE matchup_id = $1 ORDER BY id ASC", updated.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := sqlx.GetContext(ctx, h.DB, &updated.Author,
		"SELECT * FROM users WHERE id = $1", updated.AuthorID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	updated.Author.ProcessAvatarPath()

	var likesCount int64
	_ = sqlx.GetContext(ctx, h.DB, &likesCount, "SELECT COUNT(*) FROM likes WHERE matchup_id = $1", updated.ID)

	return connect.NewResponse(&matchupv1.ActivateMatchupResponse{
		Matchup: matchupToProto(h.DB, &updated, nil, likesCount),
	}), nil
}

// finalizeStandaloneMatchupIfExpiredStandalone is the non-Server-method version
// of finalizeStandaloneMatchupIfExpired for use in handler structs.
func finalizeStandaloneMatchupIfExpiredStandalone(db *sqlx.DB, matchup *models.Matchup) error {
	if matchup.BracketID != nil ||
		matchup.EndMode != "timer" ||
		matchup.EndTime == nil ||
		!isMatchupOpenStatus(matchup.Status) {
		return nil
	}
	if time.Now().Before(*matchup.EndTime) {
		return nil
	}
	if matchup.WinnerItemID == nil {
		winnerID, err := determineMatchupWinnerByVotes(matchup)
		if err != nil {
			return err
		}
		matchup.WinnerItemID = &winnerID
	}
	matchup.Status = matchupStatusCompleted
	now := time.Now()
	matchup.UpdatedAt = now
	_, err := db.ExecContext(context.Background(),
		"UPDATE matchups SET status = $1, winner_item_id = $2, updated_at = $3 WHERE id = $4",
		matchup.Status, matchup.WinnerItemID, matchup.UpdatedAt, matchup.ID)
	return err
}

// deleteMatchupCascadeStandalone is the non-Server-method version of deleteMatchupCascade.
func deleteMatchupCascadeStandalone(db *sqlx.DB, matchup *models.Matchup) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	ctx := context.Background()
	if _, err := tx.ExecContext(ctx, "DELETE FROM matchup_items WHERE matchup_id = $1", matchup.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM comments WHERE matchup_id = $1", matchup.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM likes WHERE matchup_id = $1", matchup.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM matchup_votes WHERE matchup_public_id = $1", matchup.PublicID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM matchups WHERE id = $1", matchup.ID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	_ = cache.DeleteByPrefix(ctx, "matchups:")
	userPrefix := fmt.Sprintf("user:%d:matchups:", matchup.AuthorID)
	_ = cache.DeleteByPrefix(ctx, userPrefix)
	invalidateHomeSummaryCache(matchup.AuthorID)
	if matchup.BracketID != nil {
		invalidateBracketSummaryCache(*matchup.BracketID)
	}
	return nil
}
