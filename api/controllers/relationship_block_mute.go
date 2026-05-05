package controllers

import (
	"context"
	"errors"
	"fmt"
	"time"

	userv1 "Matchup/gen/user/v1"
	"Matchup/models"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
)

// Block / Mute RPCs live on UserHandler so the UserService proto
// mounts them without a new service wiring line. Separate file
// because the code is a distinct concern from the existing user
// CRUD / follow RPCs.
//
// Semantics recap (see plan file):
//   * Block = bidirectional hide. Severs existing follow edges both
//     ways. Supersedes any active mute.
//   * Mute = one-way feed hide. Doesn't touch follows. The muted
//     user sees no change.
//   * Unblock / Unmute = DELETE row. No auto-restore of follows.

func (h *UserHandler) BlockUser(ctx context.Context, req *connect.Request[userv1.BlockUserRequest]) (*connect.Response[userv1.BlockUserResponse], error) {
	blockerID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}
	target, err := resolveUserByIdentifier(h.DB, req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	}
	if blockerID == target.ID {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("can't block yourself"))
	}

	tx, err := h.DB.Beginx()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer tx.Rollback()

	// Upsert the block — idempotent so spammy clicks don't error.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO public.user_blocks (blocker_id, blocked_id)
		VALUES ($1, $2)
		ON CONFLICT (blocker_id, blocked_id) DO NOTHING
	`, blockerID, target.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Sever existing follow edges both directions + fix the
	// denormalised counters so followers_count / following_count
	// stay honest.
	if err := severFollowEdges(ctx, tx, blockerID, target.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Block supersedes mute — if the blocker had muted this user
	// previously, the mute is redundant now. Delete it so the
	// "active mutes" list doesn't show a stale row.
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM public.user_mutes
		 WHERE muter_id = $1 AND muted_id = $2
	`, blockerID, target.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	resp := connect.NewResponse(&userv1.BlockUserResponse{Message: "user blocked"})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

func (h *UserHandler) UnblockUser(ctx context.Context, req *connect.Request[userv1.UnblockUserRequest]) (*connect.Response[userv1.UnblockUserResponse], error) {
	blockerID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}
	target, err := resolveUserByIdentifier(h.DB, req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	}
	if _, err := h.DB.ExecContext(ctx, `
		DELETE FROM public.user_blocks
		 WHERE blocker_id = $1 AND blocked_id = $2
	`, blockerID, target.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// Note: unblock does NOT restore any follow edges that the block
	// severed. Users who want to re-follow after unblocking do so
	// explicitly.
	resp := connect.NewResponse(&userv1.UnblockUserResponse{Message: "user unblocked"})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

func (h *UserHandler) MuteUser(ctx context.Context, req *connect.Request[userv1.MuteUserRequest]) (*connect.Response[userv1.MuteUserResponse], error) {
	muterID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}
	target, err := resolveUserByIdentifier(h.DB, req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	}
	if muterID == target.ID {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("can't mute yourself"))
	}
	if _, err := h.DB.ExecContext(ctx, `
		INSERT INTO public.user_mutes (muter_id, muted_id)
		VALUES ($1, $2)
		ON CONFLICT (muter_id, muted_id) DO NOTHING
	`, muterID, target.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	resp := connect.NewResponse(&userv1.MuteUserResponse{Message: "user muted"})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

func (h *UserHandler) UnmuteUser(ctx context.Context, req *connect.Request[userv1.UnmuteUserRequest]) (*connect.Response[userv1.UnmuteUserResponse], error) {
	muterID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}
	target, err := resolveUserByIdentifier(h.DB, req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	}
	if _, err := h.DB.ExecContext(ctx, `
		DELETE FROM public.user_mutes
		 WHERE muter_id = $1 AND muted_id = $2
	`, muterID, target.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	resp := connect.NewResponse(&userv1.UnmuteUserResponse{Message: "user unmuted"})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

// ListBlocks + ListMutes return the caller's active block/mute set
// paginated for the settings UI. Cursor is the last row's created_at
// (RFC3339); empty cursor means "start from newest."
func (h *UserHandler) ListBlocks(ctx context.Context, req *connect.Request[userv1.ListBlocksRequest]) (*connect.Response[userv1.ListBlocksResponse], error) {
	viewerID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}
	users, next, err := listRelationshipTargets(ctx, h.DB, "user_blocks", "blocker_id", "blocked_id", viewerID, req.Msg.GetCursor(), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	protos := make([]*userv1.UserProfile, len(users))
	for i := range users {
		u := users[i]
		protos[i] = userToProto(&u)
	}
	resp := &userv1.ListBlocksResponse{Users: protos}
	if next != "" {
		resp.NextCursor = &next
	}
	return connect.NewResponse(resp), nil
}

func (h *UserHandler) ListMutes(ctx context.Context, req *connect.Request[userv1.ListMutesRequest]) (*connect.Response[userv1.ListMutesResponse], error) {
	viewerID, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}
	users, next, err := listRelationshipTargets(ctx, h.DB, "user_mutes", "muter_id", "muted_id", viewerID, req.Msg.GetCursor(), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	protos := make([]*userv1.UserProfile, len(users))
	for i := range users {
		u := users[i]
		protos[i] = userToProto(&u)
	}
	resp := &userv1.ListMutesResponse{Users: protos}
	if next != "" {
		resp.NextCursor = &next
	}
	return connect.NewResponse(resp), nil
}

// severFollowEdges drops any existing follow row between two users
// in either direction + fixes the denormalised counters. Mirrors the
// counter logic in FollowUser / UnfollowUser. Runs inside the caller's
// transaction so block + sever land atomically.
func severFollowEdges(ctx context.Context, tx *sqlx.Tx, a, b uint) error {
	// a → b
	res, err := tx.ExecContext(ctx,
		"DELETE FROM follows WHERE follower_id = $1 AND followed_id = $2", a, b,
	)
	if err != nil {
		return fmt.Errorf("sever a→b: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		if _, err := tx.ExecContext(ctx,
			"UPDATE users SET following_count = GREATEST(following_count - 1, 0) WHERE id = $1", a,
		); err != nil {
			return fmt.Errorf("decr following_count on a: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE users SET followers_count = GREATEST(followers_count - 1, 0) WHERE id = $1", b,
		); err != nil {
			return fmt.Errorf("decr followers_count on b: %w", err)
		}
	}

	// b → a
	res, err = tx.ExecContext(ctx,
		"DELETE FROM follows WHERE follower_id = $1 AND followed_id = $2", b, a,
	)
	if err != nil {
		return fmt.Errorf("sever b→a: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		if _, err := tx.ExecContext(ctx,
			"UPDATE users SET following_count = GREATEST(following_count - 1, 0) WHERE id = $1", b,
		); err != nil {
			return fmt.Errorf("decr following_count on b: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE users SET followers_count = GREATEST(followers_count - 1, 0) WHERE id = $1", a,
		); err != nil {
			return fmt.Errorf("decr followers_count on a: %w", err)
		}
	}

	return nil
}

// listRelationshipTargets is the shared read path behind ListBlocks +
// ListMutes. Paginated by the edge's created_at timestamp so a fresh
// block sits at the top of the list.
//
// `table` / `selfColumn` / `targetColumn` parameterise the table shape
// so one helper serves both relationship types.
func listRelationshipTargets(
	ctx context.Context,
	db *sqlx.DB,
	table, selfColumn, targetColumn string,
	viewerID uint,
	cursor string,
	limit int,
) ([]models.User, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	cursorClause := ""
	args := []interface{}{viewerID}
	if cursor != "" {
		t, perr := time.Parse(time.RFC3339, cursor)
		if perr != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", perr)
		}
		cursorClause = " AND e.created_at < $2"
		args = append(args, t.UTC())
	}

	// Per-request limit+1 trick: fetch one extra row so we know if
	// there's a next page without running a second COUNT query.
	limitArg := len(args) + 1
	args = append(args, limit+1)

	query := fmt.Sprintf(`
		SELECT u.*, e.created_at AS edge_created_at
		  FROM public.%s e
		  JOIN public.users u ON u.id = e.%s
		 WHERE e.%s = $1%s
		 ORDER BY e.created_at DESC
		 LIMIT $%d
	`, table, targetColumn, selfColumn, cursorClause, limitArg)

	type row struct {
		models.User
		EdgeCreatedAt time.Time `db:"edge_created_at"`
	}
	var rows []row
	if err := sqlx.SelectContext(ctx, db, &rows, query, args...); err != nil {
		return nil, "", fmt.Errorf("list %s: %w", table, err)
	}

	nextCursor := ""
	if len(rows) > limit {
		nextCursor = rows[limit-1].EdgeCreatedAt.UTC().Format(time.RFC3339)
		rows = rows[:limit]
	}
	out := make([]models.User, len(rows))
	for i, r := range rows {
		out[i] = r.User
		out[i].ProcessAvatarPath()
	}
	return out, nextCursor, nil
}


