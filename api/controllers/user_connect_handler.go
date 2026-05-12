package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"Matchup/cache"
	appdb "Matchup/db"
	userv1 "Matchup/gen/user/v1"
	"Matchup/gen/user/v1/userv1connect"
	"Matchup/models"
	"Matchup/security"
	"Matchup/utils/fileformat"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jmoiron/sqlx"
)

// UserHandler implements userv1connect.UserServiceHandler.
type UserHandler struct {
	DB     *sqlx.DB
	ReadDB *sqlx.DB
	// S3Client is the process-wide singleton populated in
	// initializeConnectRoutes. Avatar uploads use this directly instead
	// of building a fresh client per request — see Server.S3Client.
	S3Client *s3.Client
}

var _ userv1connect.UserServiceHandler = (*UserHandler)(nil)

func (h *UserHandler) ListUsers(ctx context.Context, req *connect.Request[userv1.ListUsersRequest]) (*connect.Response[userv1.ListUsersResponse], error) {
	// Pure-read RPC — safe to serve from the replica.
	db := dbForRead(ctx, h.DB, h.ReadDB)
	var user models.User
	users, err := user.FindAllUsers(db)
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
	// Pure-read RPC — safe to serve from the replica.
	db := dbForRead(ctx, h.DB, h.ReadDB)
	user, err := resolveUserByIdentifier(db, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}

	viewerID, hasViewer := httpctx.CurrentUserID(ctx)

	// Anonymous viewers can only see public profiles. Private
	// accounts return 404 — same shape as a missing user so the
	// existence of the account isn't leaked. Once the user signs
	// in, this path naturally falls through to the followers-only
	// gate that's enforced by canViewUserContent on downstream
	// reads (matchups, brackets, likes).
	if !hasViewer && user.IsPrivate {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}

	// Block enforcement: return 404 when either side of the block
	// edge is looking up the other. Deliberately indistinguishable
	// from a genuine not-found — no "you've been blocked" leakage.
	// Admins bypass this check so moderation can still see the row.
	if hasViewer && !httpctx.IsAdminRequest(ctx) && viewerID != user.ID {
		var blockCount int
		if err := h.DB.GetContext(ctx, &blockCount, `
			SELECT COUNT(*) FROM user_blocks
			 WHERE (blocker_id = $1 AND blocked_id = $2)
			    OR (blocker_id = $2 AND blocked_id = $1)
		`, viewerID, user.ID); err == nil && blockCount > 0 {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
		}
	}

	return connect.NewResponse(&userv1.GetUserResponse{User: userToProto(user)}), nil
}

func (h *UserHandler) GetCurrentUser(ctx context.Context, req *connect.Request[userv1.GetCurrentUserRequest]) (*connect.Response[userv1.GetCurrentUserResponse], error) {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}
	// Pure-read RPC — safe to serve from the replica.
	var u models.User
	found, err := u.FindUserByID(dbForRead(ctx, h.DB, h.ReadDB), uid)
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

	// Email verification is NOT auto-sent at signup anymore. The
	// frontend pops a ConfirmModal right after register+login and
	// asks the user whether to send the link now — Yes calls
	// RequestEmailVerification, No just lands them on the page with
	// the persistent banner up. This was a deliberate UX call: the
	// previous auto-send fired into spam folders silently and users
	// thought their email had failed to register entirely. The
	// banner + write-path gate still keep them honest about
	// verifying eventually.

	// Migrate anonymous votes from this browser into the new account.
	// The frontend sends X-Anon-Id whenever localStorage.anonId is
	// set. mergeDeviceVotesToUser already handles the conflict case
	// (signup happens immediately after a vote on a matchup the user
	// hasn't seen before, so collisions are rare; the helper undoes
	// the anon vote when one is found). Same code path as the
	// existing Login-time merge. Failures here don't block signup.
	if anonID := req.Header().Get("X-Anon-Id"); anonID != "" {
		if err := mergeDeviceVotesToUser(h.DB, created.ID, anonID); err != nil {
			log.Printf("signup: merge anon votes for user %d (anon=%s): %v", created.ID, anonID, err)
		}
	}

	resp := connect.NewResponse(&userv1.CreateUserResponse{User: userToProto(created)})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
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
	resp := connect.NewResponse(&userv1.UpdateUserResponse{User: userToProto(updated)})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
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
	resp := connect.NewResponse(&userv1.UpdateUserPrivacyResponse{User: userToProto(user)})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

// defaultNotificationPrefs returns the "everything on" map. This is the
// shape migration 017 stores as the column default; we use the same map
// when decoding old rows that predate the column (NULL / empty) so the
// handler's enabled-kind check never needs a missing-key fallback.
func defaultNotificationPrefs() map[string]bool {
	return map[string]bool{
		"mention":      true,
		"engagement":   true,
		"milestone":    true,
		"prompt":       true,
		"social":       true,
		"email_digest": true,
	}
}

// decodeNotificationPrefs turns a jsonb blob into the canonical map.
// Missing keys default to true so that a user whose row was created
// before 017 (or whose prefs were partially written) still sees every
// category. Callers should NOT mutate the returned map if they plan to
// serialise it back.
func decodeNotificationPrefs(raw []byte) map[string]bool {
	prefs := defaultNotificationPrefs()
	if len(raw) == 0 {
		return prefs
	}
	var parsed map[string]bool
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return prefs
	}
	for k, v := range parsed {
		prefs[k] = v
	}
	return prefs
}

func prefsToProto(p map[string]bool) *userv1.NotificationPreferences {
	return &userv1.NotificationPreferences{
		Mention:     p["mention"],
		Engagement:  p["engagement"],
		Milestone:   p["milestone"],
		Prompt:      p["prompt"],
		Social:      p["social"],
		EmailDigest: p["email_digest"],
	}
}

// GetNotificationPreferences — viewer's own current settings. Requires
// auth; no public view.
func (h *UserHandler) GetNotificationPreferences(ctx context.Context, _ *connect.Request[userv1.GetNotificationPreferencesRequest]) (*connect.Response[userv1.GetNotificationPreferencesResponse], error) {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}

	db := dbForRead(ctx, h.DB, h.ReadDB)
	var raw []byte
	if err := db.GetContext(ctx, &raw,
		"SELECT notification_prefs FROM users WHERE id = $1", uid); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	prefs := decodeNotificationPrefs(raw)
	return connect.NewResponse(&userv1.GetNotificationPreferencesResponse{
		Prefs: prefsToProto(prefs),
	}), nil
}

// UpdateNotificationPreferences — full replacement write. Writing the
// full map (rather than a JSON-merge) keeps the server-side semantics
// simple and the client code trivial: every toggle fires a full PUT.
func (h *UserHandler) UpdateNotificationPreferences(ctx context.Context, req *connect.Request[userv1.UpdateNotificationPreferencesRequest]) (*connect.Response[userv1.UpdateNotificationPreferencesResponse], error) {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}
	in := req.Msg.GetPrefs()
	if in == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("prefs is required"))
	}

	prefs := map[string]bool{
		"mention":      in.GetMention(),
		"engagement":   in.GetEngagement(),
		"milestone":    in.GetMilestone(),
		"prompt":       in.GetPrompt(),
		"social":       in.GetSocial(),
		"email_digest": in.GetEmailDigest(),
	}
	encoded, err := json.Marshal(prefs)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if _, err := h.DB.ExecContext(ctx,
		"UPDATE users SET notification_prefs = $1::jsonb, updated_at = NOW() WHERE id = $2",
		string(encoded), uid,
	); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	resp := connect.NewResponse(&userv1.UpdateNotificationPreferencesResponse{
		Prefs: prefsToProto(prefs),
	})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
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

	// Fetch the client's pre-PUT blob from S3. CommitUploadedObject
	// validates the upload_key belongs to the caller + that the HeadObject
	// metadata matches the avatar kind's size + content-type caps; we
	// get back a byte slice ready for the existing resize pipeline.
	bucketName := bucketFromEnv()
	buf, commitErr := CommitUploadedObject(ctx, h.S3Client, bucketName, req.Msg.GetUploadKey(), UploadKindAvatar, requestorID)
	if commitErr != nil {
		return nil, commitErr
	}

	// All variants are JPEG-encoded by resizeAndUpload, so the canonical
	// filename stored in the DB always carries a .jpg extension regardless
	// of the original upload format.
	filePath := fileformat.UniqueFormat("avatar.jpg")
	keyBase := "UserProfilePics/" + filePath

	// resizeAndUpload decodes once, resizes to thumb/medium/full and uploads
	// each variant to S3. The function applies its own 30s timeout, so we
	// just hand it the request context.
	if _, err := resizeAndUpload(ctx, h.S3Client, bucketName, keyBase, buf); err != nil {
		log.Printf("S3 resize/upload failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to upload image"))
	}

	// Reap the temp upload — best effort; the S3 lifecycle rule catches
	// any leaks. Fires only on the happy path so a failed resize leaves
	// the raw blob around for debugging.
	DeleteUploadedObject(ctx, h.S3Client, bucketName, req.Msg.GetUploadKey())

	avatarUser := models.User{AvatarPath: filePath}
	updatedUser, err := avatarUser.UpdateAUserAvatar(h.DB, user.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("cannot save image"))
	}
	updatedUser.AvatarPath = appdb.ProcessAvatarPath(filePath)
	resp := connect.NewResponse(&userv1.UpdateAvatarResponse{User: userToProto(updatedUser)})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
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
	resp := connect.NewResponse(&userv1.DeleteUserResponse{Message: "user deleted"})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

func (h *UserHandler) GetFollowers(ctx context.Context, req *connect.Request[userv1.GetFollowersRequest]) (*connect.Response[userv1.GetFollowersResponse], error) {
	// Pure-read RPC — safe to serve from the replica.
	db := dbForRead(ctx, h.DB, h.ReadDB)

	target, err := resolveUserByIdentifier(db, req.Msg.Id)
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
	rows, err := fetchFollowRowsStandalone(db, "follows.followed_id = $1", []interface{}{target.ID}, "users.id = follows.follower_id", limit, cursor)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	viewerID, hasViewer := optionalViewerFromCtx(ctx)
	followingMap, followedByMap, err := loadViewerRelationships(db, viewerID, hasViewer, rows)
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
	// Pure-read RPC — safe to serve from the replica.
	db := dbForRead(ctx, h.DB, h.ReadDB)

	target, err := resolveUserByIdentifier(db, req.Msg.Id)
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

	rows, err := fetchFollowRowsStandalone(db, "follows.follower_id = $1", []interface{}{target.ID}, "users.id = follows.followed_id", limit, cursor)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	viewerID, hasViewer := optionalViewerFromCtx(ctx)
	followingMap, followedByMap, err := loadViewerRelationships(db, viewerID, hasViewer, rows)
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
	// Email-verification gate: unverified accounts can browse + vote
	// but can't add friends. Friend graphs are an abuse vector for
	// fake-account networks; gating Follow specifically (not Unfollow,
	// not the rest of the user-profile RPCs) is the lightest pressure
	// that still cuts the abuse path. Self-disables when SendGrid isn't
	// configured (see email_verification.go).
	if err := requireVerifiedEmail(ctx, h.DB, requestorID); err != nil {
		return nil, err
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

	// SSE push — the followed user's bell should refetch so the new_follower
	// item surfaces without waiting for the 60s poll tick.
	_ = cache.PublishActivity(ctx, target.ID)

	resp := connect.NewResponse(&userv1.FollowUserResponse{Message: "user followed"})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
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
	resp := connect.NewResponse(&userv1.UnfollowUserResponse{Message: "user unfollowed"})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

func (h *UserHandler) GetRelationship(ctx context.Context, req *connect.Request[userv1.GetRelationshipRequest]) (*connect.Response[userv1.GetRelationshipResponse], error) {
	requestorID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}
	// Pure-read RPC — safe to serve from the replica.
	db := dbForRead(ctx, h.DB, h.ReadDB)

	target, err := resolveUserByIdentifier(db, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}
	if requestorID == target.ID {
		return connect.NewResponse(&userv1.GetRelationshipResponse{
			Relationship: &userv1.RelationshipResponse{},
		}), nil
	}

	// One round-trip packs the 4 edge checks the UI needs: follow both
	// ways + block outgoing + mute outgoing. Incoming-block isn't part
	// of the response because GetUser already returns 404 when the
	// target has blocked the viewer — the UI never reaches this call
	// in that direction.
	var rel struct {
		Following  bool `db:"following"`
		FollowedBy bool `db:"followed_by"`
		Blocked    bool `db:"blocked"`
		Muted      bool `db:"muted"`
	}
	if err := sqlx.GetContext(ctx, db, &rel,
		`SELECT
			EXISTS(SELECT 1 FROM follows       WHERE follower_id = $1 AND followed_id = $2) AS following,
			EXISTS(SELECT 1 FROM follows       WHERE follower_id = $2 AND followed_id = $1) AS followed_by,
			EXISTS(SELECT 1 FROM user_blocks   WHERE blocker_id  = $1 AND blocked_id  = $2) AS blocked,
			EXISTS(SELECT 1 FROM user_mutes    WHERE muter_id    = $1 AND muted_id    = $2) AS muted`,
		requestorID, target.ID,
	); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&userv1.GetRelationshipResponse{
		Relationship: &userv1.RelationshipResponse{
			Following:  rel.Following,
			FollowedBy: rel.FollowedBy,
			Mutual:     rel.Following && rel.FollowedBy,
			Blocked:    rel.Blocked,
			Muted:      rel.Muted,
		},
	}), nil
}

// fetchFollowRowsStandalone is the non-Server-method version of fetchFollowRows.
func fetchFollowRowsStandalone(db *sqlx.DB, whereClause string, whereArgs []interface{}, joinClause string, limit int, cursor *followCursor) ([]followRow, error) {
	// Explicit column list (rather than `users.*`) for two reasons:
	//   1. sqlx.Select is STRICT — it errors with
	//      "missing destination name <col>" the moment the query
	//      returns a column the followRow struct doesn't declare.
	//      `users.*` returns every column on the users table, and
	//      every migration that adds a column (deleted_at, banned_at,
	//      email_verified_at, notification_prefs, …) breaks this
	//      query for any user with at least one follow edge.
	//   2. We never use the password hash on this read path, so
	//      pulling it across the wire is dead weight + a small
	//      security smell. Naming columns means we can never
	//      accidentally surface the hash to a downstream view.
	//
	// The list mirrors the followRow struct exactly. If you add a
	// field there, add the matching column here.
	const userCols = "users.id, users.public_id, users.username, users.email, " +
		"users.avatar_path, users.is_admin, users.is_private, " +
		"users.followers_count, users.following_count, " +
		"users.created_at, users.updated_at"
	query := fmt.Sprintf(
		"SELECT follows.id as follow_id, follows.created_at as follow_created_at, %s FROM follows JOIN users ON %s WHERE %s",
		userCols, joinClause, whereClause,
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
