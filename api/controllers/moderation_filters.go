package controllers

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

// Shared filter helpers for block / mute / deleted-user filtering
// across every read path that surfaces other users' content.
//
// Policy summary:
//   * Block is BIDIRECTIONAL. blockedUserIDs(viewer) returns every
//     uid the viewer either blocked OR is blocked by, so feeds +
//     comments + activity hide both directions.
//   * Mute is ONE-WAY. mutedUserIDs(muter) returns only what the
//     muter picked. Used in the muter's activity feed + push
//     delivery; muted users still see the muter.
//   * Deleted users are a different rendering concern — they stay
//     in results but their profile fields blank out server-side via
//     userToProto (see proto_mappers.go). No SQL filter here.
//
// `excludeUsersFragment` lets each caller interpolate a NOT IN (...)
// clause into its existing query without worrying about empty-slice
// edge cases or placeholder numbering.

// blockedUserIDs returns the set of user IDs the viewer has blocked
// OR who have blocked the viewer. De-duplicated. Empty slice when the
// viewer has no block edges in either direction.
//
// Cached-friendly shape (slice + stable order) so callers can memoize
// per-request if they make multiple reads.
func blockedUserIDs(ctx context.Context, db sqlx.ExtContext, viewerID uint) ([]uint, error) {
	if viewerID == 0 {
		return nil, nil
	}
	// UNION de-duplicates naturally — a pair where both users blocked
	// each other yields one row per uid in the output.
	var ids []uint
	err := sqlx.SelectContext(ctx, db, &ids, `
		SELECT blocked_id AS uid FROM public.user_blocks WHERE blocker_id = $1
		UNION
		SELECT blocker_id AS uid FROM public.user_blocks WHERE blocked_id = $1
	`, viewerID)
	if err != nil {
		return nil, fmt.Errorf("blockedUserIDs: %w", err)
	}
	return ids, nil
}

// mutedUserIDs returns the set of user IDs the muter has muted.
// One-way by design: the muter disappears from the muted user's
// perspective is NOT a thing.
func mutedUserIDs(ctx context.Context, db sqlx.ExtContext, muterID uint) ([]uint, error) {
	if muterID == 0 {
		return nil, nil
	}
	var ids []uint
	err := sqlx.SelectContext(ctx, db, &ids,
		"SELECT muted_id FROM public.user_mutes WHERE muter_id = $1",
		muterID,
	)
	if err != nil {
		return nil, fmt.Errorf("mutedUserIDs: %w", err)
	}
	return ids, nil
}

// hiddenForViewer is the common case: union of blocked + muted uids
// for feed reads where both filters apply equally (e.g. the activity
// feed). Returns de-duplicated uids. Empty slice when there's nothing
// to hide.
func hiddenForViewer(ctx context.Context, db sqlx.ExtContext, viewerID uint) ([]uint, error) {
	blocks, err := blockedUserIDs(ctx, db, viewerID)
	if err != nil {
		return nil, err
	}
	mutes, err := mutedUserIDs(ctx, db, viewerID)
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 && len(mutes) == 0 {
		return nil, nil
	}
	seen := make(map[uint]struct{}, len(blocks)+len(mutes))
	out := make([]uint, 0, len(blocks)+len(mutes))
	for _, id := range blocks {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range mutes {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

// hiddenUsernamesForViewer returns the set of usernames the viewer
// should never see in feeds — union of blocked (both directions) +
// muted targets, resolved to lowercase usernames for case-insensitive
// matching against ActivityItem.ActorUsername. Empty set = short-
// circuit (no filter needed).
//
// Takes *sqlx.DB specifically (not ExtContext) because sqlx.In's
// rebind uses a method on DB/Tx that ExtContext doesn't expose.
func hiddenUsernamesForViewer(ctx context.Context, db *sqlx.DB, viewerID uint) (map[string]bool, error) {
	ids, err := hiddenForViewer(ctx, db, viewerID)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	query, args, err := sqlx.In("SELECT LOWER(username) FROM users WHERE id IN (?)", ids)
	if err != nil {
		return nil, err
	}
	var names []string
	if err := sqlx.SelectContext(ctx, db, &names, db.Rebind(query), args...); err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set, nil
}

// excludeUsersFragment builds an "AND {column} NOT IN (...)" fragment
// starting from placeholder `$start` and incrementing. Returns empty
// string + nil args for an empty id slice (no-op — caller doesn't
// need to wrap in an if-else).
//
// Example use inside a SELECT:
//
//	frag, args := excludeUsersFragment("u.id", hidden, len(queryArgs)+1)
//	query := baseQuery + frag + " ORDER BY ..."
//	queryArgs = append(queryArgs, args...)
func excludeUsersFragment(column string, ids []uint, startPlaceholder int) (string, []interface{}) {
	if len(ids) == 0 {
		return "", nil
	}
	placeholders := make([]string, 0, len(ids))
	args := make([]interface{}, 0, len(ids))
	for i, id := range ids {
		placeholders = append(placeholders, fmt.Sprintf("$%d", startPlaceholder+i))
		args = append(args, id)
		_ = i
	}
	return fmt.Sprintf(" AND %s NOT IN (%s)", column, strings.Join(placeholders, ", ")), args
}
