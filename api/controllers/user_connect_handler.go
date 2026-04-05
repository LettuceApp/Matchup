package controllers

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	appdb "Matchup/db"
	userv1 "Matchup/gen/user/v1"
	"Matchup/gen/user/v1/userv1connect"
	"Matchup/models"
	"Matchup/security"
	"Matchup/utils/fileformat"
	httpctx "Matchup/utils/httpctx"

	aws2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
)

// UserHandler implements userv1connect.UserServiceHandler.
type UserHandler struct{ DB *sqlx.DB }

var _ userv1connect.UserServiceHandler = (*UserHandler)(nil)

func (h *UserHandler) ListUsers(ctx context.Context, req *connect.Request[userv1.ListUsersRequest]) (*connect.Response[userv1.ListUsersResponse], error) {
	var user models.User
	users, err := user.FindAllUsers(h.DB)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	protos := make([]*userv1.UserProfile, len(*users))
	for i := range *users {
		u := (*users)[i]
		protos[i] = userToProto(&u)
	}
	return connect.NewResponse(&userv1.ListUsersResponse{Users: protos}), nil
}

func (h *UserHandler) GetUser(ctx context.Context, req *connect.Request[userv1.GetUserRequest]) (*connect.Response[userv1.GetUserResponse], error) {
	user, err := resolveUserByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}
	return connect.NewResponse(&userv1.GetUserResponse{User: userToProto(user)}), nil
}

func (h *UserHandler) GetCurrentUser(ctx context.Context, req *connect.Request[userv1.GetCurrentUserRequest]) (*connect.Response[userv1.GetCurrentUserResponse], error) {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}
	var u models.User
	found, err := u.FindUserByID(h.DB, uid)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}
	return connect.NewResponse(&userv1.GetCurrentUserResponse{User: userToProto(found)}), nil
}

func (h *UserHandler) CreateUser(ctx context.Context, req *connect.Request[userv1.CreateUserRequest]) (*connect.Response[userv1.CreateUserResponse], error) {
	user := models.User{
		Username: req.Msg.Username,
		Email:    req.Msg.Email,
		Password: req.Msg.Password,
		IsAdmin:  false,
	}
	if strings.TrimSpace(user.Username) == "" {
		parts := strings.SplitN(strings.TrimSpace(user.Email), "@", 2)
		if len(parts) > 0 && parts[0] != "" {
			user.Username = parts[0]
		}
	}
	user.Prepare()
	if errs := user.Validate(""); len(errs) > 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%v", errs))
	}
	created, err := user.SaveUser(h.DB)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "uni_users_username") || strings.Contains(msg, "users_username") {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("username already taken"))
		}
		if strings.Contains(msg, "uni_users_email") || strings.Contains(msg, "users_email") {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("email already registered"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&userv1.CreateUserResponse{User: userToProto(created)}), nil
}

func (h *UserHandler) UpdateUser(ctx context.Context, req *connect.Request[userv1.UpdateUserRequest]) (*connect.Response[userv1.UpdateUserResponse], error) {
	user, err := resolveUserByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}
	requestorID, ok := httpctx.CurrentUserID(ctx)
	if !ok || (requestorID != user.ID && !httpctx.IsAdminRequest(ctx)) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("unauthorized"))
	}

	var formerUser models.User
	if err := sqlx.GetContext(ctx, h.DB, &formerUser, "SELECT * FROM users WHERE id = $1", user.ID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}

	newUser := models.User{Username: formerUser.Username}
	if req.Msg.CurrentPassword != nil && req.Msg.NewPassword != nil {
		if len(*req.Msg.NewPassword) < 6 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("password must be at least 6 characters"))
		}
		if err := security.VerifyPassword(formerUser.Password, *req.Msg.CurrentPassword); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("current password is incorrect"))
		}
		newUser.Password = *req.Msg.NewPassword
	}
	if req.Msg.Email != nil {
		newUser.Email = *req.Msg.Email
	} else {
		newUser.Email = formerUser.Email
	}
	if req.Msg.Bio != nil {
		newUser.Bio = *req.Msg.Bio
	} else {
		newUser.Bio = formerUser.Bio
	}

	newUser.Prepare()
	if errs := newUser.Validate("update"); len(errs) > 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%v", errs))
	}
	updated, err := newUser.UpdateAUser(h.DB, user.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&userv1.UpdateUserResponse{User: userToProto(updated)}), nil
}

func (h *UserHandler) UpdateUserPrivacy(ctx context.Context, req *connect.Request[userv1.UpdateUserPrivacyRequest]) (*connect.Response[userv1.UpdateUserPrivacyResponse], error) {
	user, err := resolveUserByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}
	requestorID, ok := httpctx.CurrentUserID(ctx)
	if !ok || (requestorID != user.ID && !httpctx.IsAdminRequest(ctx)) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("unauthorized"))
	}
	if _, err := h.DB.ExecContext(ctx, "UPDATE users SET is_private = $1 WHERE id = $2", req.Msg.IsPrivate, user.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	user.IsPrivate = req.Msg.IsPrivate
	return connect.NewResponse(&userv1.UpdateUserPrivacyResponse{User: userToProto(user)}), nil
}

func (h *UserHandler) UpdateAvatar(ctx context.Context, req *connect.Request[userv1.UpdateAvatarRequest]) (*connect.Response[userv1.UpdateAvatarResponse], error) {
	user, err := resolveUserByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}
	requestorID, ok := httpctx.CurrentUserID(ctx)
	if !ok || (requestorID != user.ID && !httpctx.IsAdminRequest(ctx)) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("unauthorized"))
	}

	buf := req.Msg.AvatarData
	if len(buf) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("avatar_data is required"))
	}
	if len(buf) > 512_000 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("file too large (max 500KB)"))
	}
	fileType := http.DetectContentType(buf)
	if !strings.HasPrefix(fileType, "image/") {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("not an image"))
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
	filePath := fileformat.UniqueFormat("avatar" + ext)
	key := "UserProfilePics/" + filePath

	rawBucket := os.Getenv("S3_BUCKET")
	bucketName := strings.SplitN(rawBucket, "/", 2)[0]
	if bucketName == "" {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("server configuration error"))
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-2"
	}

	var cfg aws2.Config
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")
	if accessKey != "" && secretKey != "" {
		cfg, err = config.LoadDefaultConfig(context.TODO(),
			config.WithRegion(region),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, sessionToken)),
		)
	} else {
		cfg, err = config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("AWS configuration error: %w", err))
	}

	s3Client := s3.NewFromConfig(cfg)
	size := int64(len(buf))
	if _, err := s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:        aws2.String(bucketName),
		Key:           aws2.String(key),
		Body:          bytes.NewReader(buf),
		ContentLength: aws2.Int64(size),
		ContentType:   aws2.String(fileType),
	}); err != nil {
		log.Printf("S3 upload failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to upload image"))
	}

	avatarUser := models.User{AvatarPath: filePath}
	updatedUser, err := avatarUser.UpdateAUserAvatar(h.DB, user.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("cannot save image"))
	}
	updatedUser.AvatarPath = appdb.ProcessAvatarPath(filePath)
	return connect.NewResponse(&userv1.UpdateAvatarResponse{User: userToProto(updatedUser)}), nil
}

func (h *UserHandler) DeleteUser(ctx context.Context, req *connect.Request[userv1.DeleteUserRequest]) (*connect.Response[userv1.DeleteUserResponse], error) {
	user, err := resolveUserByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}
	requestorID, ok := httpctx.CurrentUserID(ctx)
	if !ok || (requestorID != user.ID && !httpctx.IsAdminRequest(ctx)) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("unauthorized"))
	}

	tx, err := h.DB.Beginx()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer tx.Rollback()

	if err := removeUserFollowEdges(tx, user.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	var u models.User
	if _, err := u.DeleteAUser(tx, user.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&userv1.DeleteUserResponse{Message: "user deleted"}), nil
}

func (h *UserHandler) GetFollowers(ctx context.Context, req *connect.Request[userv1.GetFollowersRequest]) (*connect.Response[userv1.GetFollowersResponse], error) {
	target, err := resolveUserByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}

	limit := 20
	if req.Msg.Limit != nil {
		limit = parseLimit(fmt.Sprintf("%d", *req.Msg.Limit))
	}
	var cursor *followCursor
	if req.Msg.Cursor != nil && *req.Msg.Cursor != "" {
		c, err := parseFollowCursor(*req.Msg.Cursor)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cursor"))
		}
		cursor = c
	}

	// fetchFollowRows is a Server method — use standalone query here
	rows, err := fetchFollowRowsStandalone(h.DB, "follows.followed_id = $1", []interface{}{target.ID}, "users.id = follows.follower_id", limit, cursor)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	viewerID, hasViewer := optionalViewerFromCtx(ctx)
	followingMap, followedByMap, err := loadViewerRelationships(h.DB, viewerID, hasViewer, rows)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	dtos, nextCursor := buildFollowListResponse(rows, limit, hasViewer, followingMap, followedByMap)

	protos := make([]*userv1.FollowListUser, len(dtos))
	for i, dto := range dtos {
		protos[i] = followListUserToProto(dto)
	}
	resp := &userv1.GetFollowersResponse{Users: protos}
	resp.NextCursor = nextCursor
	return connect.NewResponse(resp), nil
}

func (h *UserHandler) GetFollowing(ctx context.Context, req *connect.Request[userv1.GetFollowingRequest]) (*connect.Response[userv1.GetFollowingResponse], error) {
	target, err := resolveUserByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}

	limit := 20
	if req.Msg.Limit != nil {
		limit = parseLimit(fmt.Sprintf("%d", *req.Msg.Limit))
	}
	var cursor *followCursor
	if req.Msg.Cursor != nil && *req.Msg.Cursor != "" {
		c, err := parseFollowCursor(*req.Msg.Cursor)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cursor"))
		}
		cursor = c
	}

	rows, err := fetchFollowRowsStandalone(h.DB, "follows.follower_id = $1", []interface{}{target.ID}, "users.id = follows.followed_id", limit, cursor)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	viewerID, hasViewer := optionalViewerFromCtx(ctx)
	followingMap, followedByMap, err := loadViewerRelationships(h.DB, viewerID, hasViewer, rows)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	dtos, nextCursor := buildFollowListResponse(rows, limit, hasViewer, followingMap, followedByMap)

	protos := make([]*userv1.FollowListUser, len(dtos))
	for i, dto := range dtos {
		protos[i] = followListUserToProto(dto)
	}
	resp := &userv1.GetFollowingResponse{Users: protos}
	resp.NextCursor = nextCursor
	return connect.NewResponse(resp), nil
}

func (h *UserHandler) FollowUser(ctx context.Context, req *connect.Request[userv1.FollowUserRequest]) (*connect.Response[userv1.FollowUserResponse], error) {
	requestorID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}
	target, err := resolveUserByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}
	if requestorID == target.ID {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cannot follow yourself"))
	}

	tx, err := h.DB.Beginx()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, "INSERT INTO follows (follower_id, followed_id, created_at) VALUES ($1, $2, NOW()) ON CONFLICT DO NOTHING", requestorID, target.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if rows, _ := result.RowsAffected(); rows > 0 {
		if _, err := tx.ExecContext(ctx, "UPDATE users SET following_count = following_count + 1 WHERE id = $1", requestorID); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE users SET followers_count = followers_count + 1 WHERE id = $1", target.ID); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&userv1.FollowUserResponse{Message: "user followed"}), nil
}

func (h *UserHandler) UnfollowUser(ctx context.Context, req *connect.Request[userv1.UnfollowUserRequest]) (*connect.Response[userv1.UnfollowUserResponse], error) {
	requestorID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}
	target, err := resolveUserByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}
	if requestorID == target.ID {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cannot unfollow yourself"))
	}

	tx, err := h.DB.Beginx()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, "DELETE FROM follows WHERE follower_id = $1 AND followed_id = $2", requestorID, target.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if rows, _ := result.RowsAffected(); rows > 0 {
		if _, err := tx.ExecContext(ctx, "UPDATE users SET following_count = GREATEST(following_count - 1, 0) WHERE id = $1", requestorID); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE users SET followers_count = GREATEST(followers_count - 1, 0) WHERE id = $1", target.ID); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&userv1.UnfollowUserResponse{Message: "user unfollowed"}), nil
}

func (h *UserHandler) GetRelationship(ctx context.Context, req *connect.Request[userv1.GetRelationshipRequest]) (*connect.Response[userv1.GetRelationshipResponse], error) {
	requestorID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}
	target, err := resolveUserByIdentifier(h.DB, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}
	if requestorID == target.ID {
		return connect.NewResponse(&userv1.GetRelationshipResponse{
			Relationship: &userv1.RelationshipResponse{},
		}), nil
	}

	var rel struct {
		Following  bool `db:"following"`
		FollowedBy bool `db:"followed_by"`
	}
	if err := sqlx.GetContext(ctx, h.DB, &rel,
		`SELECT
			EXISTS(SELECT 1 FROM follows WHERE follower_id = $1 AND followed_id = $2) AS following,
			EXISTS(SELECT 1 FROM follows WHERE follower_id = $3 AND followed_id = $4) AS followed_by`,
		requestorID, target.ID, target.ID, requestorID,
	); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&userv1.GetRelationshipResponse{
		Relationship: &userv1.RelationshipResponse{
			Following:  rel.Following,
			FollowedBy: rel.FollowedBy,
			Mutual:     rel.Following && rel.FollowedBy,
		},
	}), nil
}

// fetchFollowRowsStandalone is the non-Server-method version of fetchFollowRows.
func fetchFollowRowsStandalone(db *sqlx.DB, whereClause string, whereArgs []interface{}, joinClause string, limit int, cursor *followCursor) ([]followRow, error) {
	query := fmt.Sprintf(
		"SELECT follows.id as follow_id, follows.created_at as follow_created_at, users.* FROM follows JOIN users ON %s WHERE %s",
		joinClause, whereClause,
	)
	args := make([]interface{}, len(whereArgs))
	copy(args, whereArgs)
	paramIdx := len(args) + 1

	if cursor != nil {
		query += fmt.Sprintf(
			" AND ((follows.created_at < $%d) OR (follows.created_at = $%d AND follows.id < $%d))",
			paramIdx, paramIdx+1, paramIdx+2,
		)
		args = append(args, cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
		paramIdx += 3
	}
	query += fmt.Sprintf(" ORDER BY follows.created_at DESC, follows.id DESC LIMIT $%d", paramIdx)
	args = append(args, limit+1)

	var rows []followRow
	if err := sqlx.SelectContext(context.Background(), db, &rows, query, args...); err != nil {
		return nil, err
	}
	return rows, nil
}
