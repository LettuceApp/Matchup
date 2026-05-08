package controllers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"Matchup/cache"
	commentv1 "Matchup/gen/comment/v1"
	"Matchup/gen/comment/v1/commentv1connect"
	"Matchup/middlewares"
	"Matchup/models"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
)

// truncateForPush keeps the body short enough to fit inside the
// native notification row across iOS Safari (~160 char cap) and
// desktop Chrome (~300 char cap) without being cut mid-word.
func truncateForPush(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	trimmed := s[:limit]
	// Avoid cutting in the middle of a word when there's a nearby
	// space we can break on.
	if idx := strings.LastIndex(trimmed, " "); idx > limit-30 {
		trimmed = trimmed[:idx]
	}
	return trimmed + "…"
}

// Comment rate-limit budget: 10 comments per minute per authenticated
// user, shared across matchup + bracket comment creation. Tuned for
// "normal bursty usage" (5-6 replies in a thread) while stopping bots.
const (
	commentRateBucket = "comment"
	commentRateBudget = 10
	commentRateWindow = time.Minute
)

func commentRateLimitError() error {
	return connect.NewError(
		connect.CodeResourceExhausted,
		errors.New("You're commenting too fast. Give it a minute and try again."),
	)
}

// CommentHandler implements commentv1connect.CommentServiceHandler.
type CommentHandler struct {
	DB     *sqlx.DB
	ReadDB *sqlx.DB
}

var _ commentv1connect.CommentServiceHandler = (*CommentHandler)(nil)

// GetComments returns all comments for a matchup.
func (h *CommentHandler) GetComments(ctx context.Context, req *connect.Request[commentv1.GetCommentsRequest]) (*connect.Response[commentv1.GetCommentsResponse], error) {
	// Pure-read RPC — safe to serve from the replica.
	db := dbForRead(ctx, h.DB, h.ReadDB)

	matchupRecord, err := resolveMatchupByIdentifier(db, req.Msg.MatchupId)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid matchup id"))
		}
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("matchup not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to retrieve matchup"))
	}

	matchup := models.Matchup{}
	if err := sqlx.GetContext(ctx, db, &matchup, "SELECT * FROM matchups WHERE id = $1", matchupRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("matchup not found"))
	}
	if err := sqlx.GetContext(ctx, db, &matchup.Author, "SELECT * FROM users WHERE id = $1", matchup.AuthorID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("matchup not found"))
	}
	matchup.Author.ProcessAvatarPath()

	viewerID, hasViewer := optionalViewerFromCtx(ctx)
	allowed, reason, err := canViewUserContent(db, viewerID, hasViewer, &matchup.Author, matchup.Visibility, httpctx.IsAdminRequest(ctx))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to check matchup visibility"))
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("%s", visibilityErrorMessage(reason)))
	}

	comments := []models.Comment{}
	if err := sqlx.SelectContext(ctx, db, &comments,
		"SELECT * FROM comments WHERE matchup_id = $1 ORDER BY created_at DESC", matchupRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to retrieve comments"))
	}

	// Batch-load authors for comments.
	if len(comments) > 0 {
		authorIDs := make([]uint, len(comments))
		for i, c := range comments {
			authorIDs[i] = c.UserID
		}
		query, args, err := sqlx.In("SELECT * FROM users WHERE id IN (?)", authorIDs)
		if err == nil {
			query = db.Rebind(query)
			var authors []models.User
			if err := sqlx.SelectContext(ctx, db, &authors, query, args...); err == nil {
				authorMap := make(map[uint]models.User, len(authors))
				for _, a := range authors {
					a.ProcessAvatarPath()
					authorMap[a.ID] = a
				}
				for i := range comments {
					if a, ok := authorMap[comments[i].UserID]; ok {
						comments[i].Author = a
					}
				}
			}
		}
	}

	protos := make([]*commentv1.CommentData, len(comments))
	for i, c := range comments {
		protos[i] = commentToProto(db, c)
	}

	return connect.NewResponse(&commentv1.GetCommentsResponse{Comments: protos}), nil
}

// CreateComment adds a comment to a matchup.
func (h *CommentHandler) CreateComment(ctx context.Context, req *connect.Request[commentv1.CreateCommentRequest]) (*connect.Response[commentv1.CreateCommentResponse], error) {
	matchupRecord, err := resolveMatchupByIdentifier(h.DB, req.Msg.MatchupId)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid matchup id"))
		}
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("matchup not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to retrieve matchup"))
	}

	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}

	// Rate limit before we touch the DB — a blocked attempt shouldn't
	// cost a users-table SELECT or a visibility check.
	if allowed, _ := middlewares.CheckUserRateLimit(ctx, uid, commentRateBucket, commentRateBudget, commentRateWindow); !allowed {
		return nil, commentRateLimitError()
	}

	user := models.User{}
	if err := sqlx.GetContext(ctx, h.DB, &user, "SELECT * FROM users WHERE id = $1", uid); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("user not found"))
	}

	matchup := models.Matchup{}
	if err := sqlx.GetContext(ctx, h.DB, &matchup, "SELECT * FROM matchups WHERE id = $1", matchupRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("matchup not found"))
	}
	if err := sqlx.GetContext(ctx, h.DB, &matchup.Author, "SELECT * FROM users WHERE id = $1", matchup.AuthorID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("matchup not found"))
	}
	matchup.Author.ProcessAvatarPath()

	allowed, reason, err := canViewUserContent(h.DB, uid, true, &matchup.Author, matchup.Visibility, httpctx.IsAdminRequest(ctx))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to check matchup visibility"))
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("%s", visibilityErrorMessage(reason)))
	}

	isOwner := uid == matchup.AuthorID
	if !isOwner {
		// Comment creation intentionally NOT gated by requireVerifiedEmail
		// — comments are an engagement surface and gating them on a
		// pre-verify state silences too many real first-time users.
		// Bracket-create + follow-user are the gated surfaces; comment
		// content is moderated through Report instead.

		if matchup.Status == matchupStatusDraft {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("matchup is not active"))
		}

		if matchup.BracketID != nil && matchup.Status != matchupStatusCompleted {
			var bracket models.Bracket
			if err := sqlx.GetContext(ctx, h.DB, &bracket, "SELECT * FROM brackets WHERE id = $1", *matchup.BracketID); err != nil {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("bracket not found"))
			}
			if bracket.Status != "active" {
				return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("bracket is not active"))
			}
			if matchup.Round == nil || *matchup.Round != bracket.CurrentRound {
				return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("matchup is not in the active round"))
			}
		}
	}

	comment := models.Comment{
		Body: req.Msg.Body,
	}
	comment.UserID = uid
	comment.MatchupID = matchupRecord.ID

	comment.Prepare()
	if errs := comment.Validate(""); len(errs) > 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%v", errs))
	}

	commentCreated, err := comment.SaveComment(h.DB)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to save comment"))
	}

	commentCreated.Author = user

	invalidateHomeSummaryCache(matchup.AuthorID)

	// SSE push — ping the matchup author (skip self-comments) and every
	// @mentioned user whose handle resolves to a real account. The
	// payload is just "ping" so clients refetch; we don't need to
	// decide kinds on the publish side.
	if !isOwner {
		_ = cache.PublishActivity(ctx, matchup.AuthorID)
	}
	mentioned := resolveMentionedUserIDs(ctx, h.DB, req.Msg.Body, uid)
	for _, mentionedID := range mentioned {
		_ = cache.PublishActivity(ctx, mentionedID)
	}

	// Web Push — mentions are a priority kind, so we fire a native
	// browser notification to each mentioned user on top of the SSE
	// ping. Matchup / bracket authors don't get pushed here — they
	// get a comment_received row in the feed but that's not on the
	// priority list per product scope.
	pushURL := fmt.Sprintf("%s/users/%s/matchup/%s",
		strings.TrimRight(mailerAppBaseURL(), "/"),
		matchup.Author.Username, matchup.PublicID)
	for _, mentionedID := range mentioned {
		_ = cache.PublishPushToUser(ctx, h.DB, mentionedID, cache.PushPayload{
			Title: "@" + user.Username + " mentioned you",
			Body:  truncateForPush(req.Msg.Body, 140),
			URL:   pushURL,
			Tag:   fmt.Sprintf("mention:%s", matchup.PublicID),
		})
	}

	resp := connect.NewResponse(&commentv1.CreateCommentResponse{
		Comment: commentToProto(h.DB, *commentCreated),
	})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

// UpdateComment updates the body of a matchup comment.
func (h *CommentHandler) UpdateComment(ctx context.Context, req *connect.Request[commentv1.UpdateCommentRequest]) (*connect.Response[commentv1.UpdateCommentResponse], error) {
	commentRecord, err := resolveCommentByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid comment id"))
		}
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("comment not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("error finding comment"))
	}

	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}

	origComment := models.Comment{}
	if err := sqlx.GetContext(ctx, h.DB, &origComment, "SELECT * FROM comments WHERE id = $1", commentRecord.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("comment not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("error finding comment"))
	}

	if uid != origComment.UserID && !httpctx.IsAdminRequest(ctx) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("unauthorized"))
	}

	comment := models.Comment{
		Body: req.Msg.Body,
	}
	comment.Prepare()
	if errs := comment.Validate(""); len(errs) > 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%v", errs))
	}

	comment.ID = origComment.ID
	comment.UserID = origComment.UserID
	comment.MatchupID = origComment.MatchupID

	commentUpdated, err := comment.UpdateAComment(h.DB)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to update comment"))
	}

	var author models.User
	if err := sqlx.GetContext(ctx, h.DB, &author, "SELECT * FROM users WHERE id = $1", commentUpdated.UserID); err == nil {
		author.ProcessAvatarPath()
		commentUpdated.Author = author
	}

	resp := connect.NewResponse(&commentv1.UpdateCommentResponse{
		Comment: commentToProto(h.DB, *commentUpdated),
	})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

// DeleteComment deletes a matchup comment.
func (h *CommentHandler) DeleteComment(ctx context.Context, req *connect.Request[commentv1.DeleteCommentRequest]) (*connect.Response[commentv1.DeleteCommentResponse], error) {
	commentRecord, err := resolveCommentByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid comment id"))
		}
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("comment not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("error finding comment"))
	}

	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}

	comment := models.Comment{}
	if err := sqlx.GetContext(ctx, h.DB, &comment, "SELECT * FROM comments WHERE id = $1", commentRecord.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("comment not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("error finding comment"))
	}

	if uid != comment.UserID && !httpctx.IsAdminRequest(ctx) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("unauthorized"))
	}

	if _, err := comment.DeleteAComment(h.DB); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to delete comment"))
	}

	var matchup models.Matchup
	if err := sqlx.GetContext(ctx, h.DB, &matchup, "SELECT * FROM matchups WHERE id = $1", comment.MatchupID); err == nil {
		invalidateHomeSummaryCache(matchup.AuthorID)
	}

	resp := connect.NewResponse(&commentv1.DeleteCommentResponse{Message: "comment deleted"})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

// GetBracketComments returns all comments for a bracket.
func (h *CommentHandler) GetBracketComments(ctx context.Context, req *connect.Request[commentv1.GetBracketCommentsRequest]) (*connect.Response[commentv1.GetBracketCommentsResponse], error) {
	// Pure-read RPC — safe to serve from the replica.
	db := dbForRead(ctx, h.DB, h.ReadDB)

	bracketRecord, err := resolveBracketByIdentifier(db, req.Msg.BracketId)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid bracket id"))
		}
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("bracket not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to retrieve bracket"))
	}

	bracket := models.Bracket{}
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
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to check bracket visibility"))
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("%s", visibilityErrorMessage(reason)))
	}

	comments := []models.BracketComment{}
	if err := sqlx.SelectContext(ctx, db, &comments,
		"SELECT * FROM bracket_comments WHERE bracket_id = $1 ORDER BY created_at DESC", bracketRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to retrieve comments"))
	}

	// Batch-load authors for comments.
	if len(comments) > 0 {
		authorIDs := make([]uint, len(comments))
		for i, c := range comments {
			authorIDs[i] = c.UserID
		}
		query, args, err := sqlx.In("SELECT * FROM users WHERE id IN (?)", authorIDs)
		if err == nil {
			query = db.Rebind(query)
			var authors []models.User
			if err := sqlx.SelectContext(ctx, db, &authors, query, args...); err == nil {
				authorMap := make(map[uint]models.User, len(authors))
				for _, a := range authors {
					a.ProcessAvatarPath()
					authorMap[a.ID] = a
				}
				for i := range comments {
					if a, ok := authorMap[comments[i].UserID]; ok {
						comments[i].Author = a
					}
				}
			}
		}
	}

	protos := make([]*commentv1.BracketCommentData, len(comments))
	for i, c := range comments {
		protos[i] = bracketCommentToProto(db, c)
	}

	return connect.NewResponse(&commentv1.GetBracketCommentsResponse{Comments: protos}), nil
}

// CreateBracketComment adds a comment to a bracket.
func (h *CommentHandler) CreateBracketComment(ctx context.Context, req *connect.Request[commentv1.CreateBracketCommentRequest]) (*connect.Response[commentv1.CreateBracketCommentResponse], error) {
	bracketRecord, err := resolveBracketByIdentifier(h.DB, req.Msg.BracketId)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid bracket id"))
		}
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("bracket not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to retrieve bracket"))
	}

	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}

	// Same shared comment bucket as CreateComment — 10/min per user
	// across matchup + bracket comments. Intentional: a bot targeting
	// brackets and matchups on alternating calls shouldn't double its
	// effective budget.
	if allowed, _ := middlewares.CheckUserRateLimit(ctx, uid, commentRateBucket, commentRateBudget, commentRateWindow); !allowed {
		return nil, commentRateLimitError()
	}

	user := models.User{}
	if err := sqlx.GetContext(ctx, h.DB, &user, "SELECT * FROM users WHERE id = $1", uid); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("user not found"))
	}

	bracket := models.Bracket{}
	if err := sqlx.GetContext(ctx, h.DB, &bracket, "SELECT * FROM brackets WHERE id = $1", bracketRecord.ID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("bracket not found"))
	}
	if err := sqlx.GetContext(ctx, h.DB, &bracket.Author, "SELECT * FROM users WHERE id = $1", bracket.AuthorID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("bracket not found"))
	}
	bracket.Author.ProcessAvatarPath()

	allowed, reason, err := canViewUserContent(h.DB, uid, true, &bracket.Author, bracket.Visibility, httpctx.IsAdminRequest(ctx))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to check bracket visibility"))
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("%s", visibilityErrorMessage(reason)))
	}

	// Bracket-comment creation intentionally NOT gated by
	// requireVerifiedEmail — same rationale as matchup comments.

	comment := models.BracketComment{
		Body: req.Msg.Body,
	}
	comment.UserID = uid
	comment.BracketID = bracketRecord.ID

	comment.Prepare()
	if errs := comment.Validate(""); len(errs) > 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%v", errs))
	}

	commentCreated, err := comment.SaveComment(h.DB)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to save comment"))
	}

	commentCreated.Author = user

	invalidateHomeSummaryCache(bracket.AuthorID)

	// SSE push — same shape as CreateComment (author + mentions).
	if uid != bracket.AuthorID {
		_ = cache.PublishActivity(ctx, bracket.AuthorID)
	}
	mentioned := resolveMentionedUserIDs(ctx, h.DB, req.Msg.Body, uid)
	for _, mentionedID := range mentioned {
		_ = cache.PublishActivity(ctx, mentionedID)
	}

	// Web Push — mentions are a priority kind. Link lands on the bracket.
	pushURL := fmt.Sprintf("%s/brackets/%s",
		strings.TrimRight(mailerAppBaseURL(), "/"), bracket.PublicID)
	for _, mentionedID := range mentioned {
		_ = cache.PublishPushToUser(ctx, h.DB, mentionedID, cache.PushPayload{
			Title: "@" + user.Username + " mentioned you",
			Body:  truncateForPush(req.Msg.Body, 140),
			URL:   pushURL,
			Tag:   fmt.Sprintf("mention:%s", bracket.PublicID),
		})
	}

	resp := connect.NewResponse(&commentv1.CreateBracketCommentResponse{
		Comment: bracketCommentToProto(h.DB, *commentCreated),
	})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

// DeleteBracketComment deletes a bracket comment.
func (h *CommentHandler) DeleteBracketComment(ctx context.Context, req *connect.Request[commentv1.DeleteBracketCommentRequest]) (*connect.Response[commentv1.DeleteBracketCommentResponse], error) {
	commentRecord, err := resolveBracketCommentByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid comment id"))
		}
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("comment not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("error finding comment"))
	}

	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}

	comment := models.BracketComment{}
	if err := sqlx.GetContext(ctx, h.DB, &comment, "SELECT * FROM bracket_comments WHERE id = $1", commentRecord.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("comment not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("error finding comment"))
	}

	if uid != comment.UserID && !httpctx.IsAdminRequest(ctx) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("unauthorized"))
	}

	if _, err := comment.DeleteAComment(h.DB); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to delete comment"))
	}

	var bracket models.Bracket
	if err := sqlx.GetContext(ctx, h.DB, &bracket, "SELECT * FROM brackets WHERE id = $1", comment.BracketID); err == nil {
		invalidateHomeSummaryCache(bracket.AuthorID)
	}

	resp := connect.NewResponse(&commentv1.DeleteBracketCommentResponse{Message: "comment deleted"})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}
