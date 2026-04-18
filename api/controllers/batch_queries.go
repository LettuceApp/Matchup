package controllers

import (
	"context"
	"fmt"
	"strconv"

	"Matchup/cache"
	"Matchup/models"

	"github.com/jmoiron/sqlx"
)

// getBracketsByIDs batch-fetches brackets by internal IDs.
func getBracketsByIDs(db sqlx.ExtContext, ids []uint) map[uint]*models.Bracket {
	if len(ids) == 0 {
		return map[uint]*models.Bracket{}
	}
	query, args, err := sqlx.In("SELECT * FROM brackets WHERE id IN (?)", ids)
	if err != nil {
		return map[uint]*models.Bracket{}
	}
	var rows []models.Bracket
	if err := sqlx.SelectContext(context.Background(), db, &rows, db.Rebind(query), args...); err != nil {
		return map[uint]*models.Bracket{}
	}
	result := make(map[uint]*models.Bracket, len(rows))
	for i := range rows {
		b := rows[i]
		result[b.ID] = &b
	}
	return result
}

// getUsersByIDs batch-fetches users by internal IDs with avatar paths resolved.
func getUsersByIDs(db sqlx.ExtContext, ids []uint) map[uint]*models.User {
	if len(ids) == 0 {
		return map[uint]*models.User{}
	}
	query, args, err := sqlx.In("SELECT * FROM users WHERE id IN (?)", ids)
	if err != nil {
		return map[uint]*models.User{}
	}
	var rows []models.User
	if err := sqlx.SelectContext(context.Background(), db, &rows, db.Rebind(query), args...); err != nil {
		return map[uint]*models.User{}
	}
	result := make(map[uint]*models.User, len(rows))
	for i := range rows {
		u := rows[i]
		u.ProcessAvatarPath()
		result[u.ID] = &u
	}
	return result
}

// getMatchupsByIDs batch-fetches matchups by internal IDs.
func getMatchupsByIDs(db sqlx.ExtContext, ids []uint) map[uint]*models.Matchup {
	if len(ids) == 0 {
		return map[uint]*models.Matchup{}
	}
	query, args, err := sqlx.In("SELECT * FROM matchups WHERE id IN (?)", ids)
	if err != nil {
		return map[uint]*models.Matchup{}
	}
	var rows []models.Matchup
	if err := sqlx.SelectContext(context.Background(), db, &rows, db.Rebind(query), args...); err != nil {
		return map[uint]*models.Matchup{}
	}
	result := make(map[uint]*models.Matchup, len(rows))
	for i := range rows {
		m := rows[i]
		result[m.ID] = &m
	}
	return result
}

// Note: getBracketLikesCounts / getBracketCommentsCounts / getMatchupLikesCounts
// were removed in migration 006. The counts now live on the brackets/matchups
// rows themselves (LikesCount, CommentsCount), maintained by triggers. Read
// them directly off the loaded model instead of issuing a batch COUNT query.

// voteDeltaKey returns the Redis key used to buffer pending vote count
// changes for a single matchup_item. The flush worker
// (jobs/handlers/flush_votes.go) reads these keys, GETSETs them to
// zero, and applies the delta to matchup_items.votes in Postgres.
//
// Keep in lockstep with the worker — both sides use the same prefix.
func voteDeltaKey(itemID uint) string {
	return fmt.Sprintf("%s%d", cache.VoteDeltaKeyPrefix, itemID)
}

// VoteDeltaKeyPrefix re-exports the canonical constant from the cache
// package so existing tests and callers within controllers/ still compile.
const VoteDeltaKeyPrefix = cache.VoteDeltaKeyPrefix

// loadVoteDeltas batch-fetches the pending Redis vote deltas for a set
// of matchup_item IDs and returns a map of item_id → delta. Missing
// keys mean "no pending delta" and are returned as 0.
//
// On any Redis failure (or if Redis is not configured at all) the
// helper returns an empty map. Callers should treat this as "no
// pending votes" — read endpoints fall back to whatever the database
// has, which is correct (just possibly slightly stale).
func loadVoteDeltas(ctx context.Context, ids []uint) map[uint]int64 {
	result := make(map[uint]int64, len(ids))
	if len(ids) == 0 || cache.Client == nil {
		return result
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = voteDeltaKey(id)
	}
	values, err := cache.Client.MGet(ctx, keys...).Result()
	if err != nil {
		return result
	}
	for i, raw := range values {
		if raw == nil {
			continue
		}
		s, ok := raw.(string)
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			continue
		}
		if n != 0 {
			result[ids[i]] = n
		}
	}
	return result
}

// applyVoteDeltasToItems mutates a slice of MatchupItem in place,
// adding pending Redis deltas to each item's Votes count. Use this at
// the boundary of any read endpoint that returns matchup items so
// clients see live counts even when the flush worker hasn't fired yet.
//
// votes is stored as int in the model (and capped at zero — votes
// never go negative for a presented total).
func applyVoteDeltasToItems(ctx context.Context, items []models.MatchupItem) {
	if len(items) == 0 {
		return
	}
	ids := make([]uint, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	deltas := loadVoteDeltas(ctx, ids)
	if len(deltas) == 0 {
		return
	}
	for i := range items {
		if d, ok := deltas[items[i].ID]; ok {
			adjusted := int64(items[i].Votes) + d
			if adjusted < 0 {
				adjusted = 0
			}
			items[i].Votes = int(adjusted)
		}
	}
}

// applyVoteDeltaToItem is the single-item version. Used by VoteItem
// after committing the vote, so the response reflects the brand-new
// count without waiting for the flush worker.
func applyVoteDeltaToItem(ctx context.Context, item *models.MatchupItem) {
	if item == nil {
		return
	}
	deltas := loadVoteDeltas(ctx, []uint{item.ID})
	if d, ok := deltas[item.ID]; ok {
		adjusted := int64(item.Votes) + d
		if adjusted < 0 {
			adjusted = 0
		}
		item.Votes = int(adjusted)
	}
}

// applyVoteDeltasToMatchups walks every matchup in the slice and merges
// pending Redis vote deltas onto each matchup's Items slice.
//
// This is the helper to use whenever you have a `[]models.Matchup`
// whose Items have been hydrated and you want live counts. It collects
// all item IDs across all matchups and issues a single MGET, so the
// per-call cost is O(1) regardless of how many matchups are in the
// list — much cheaper than calling applyVoteDeltasToItems per matchup.
func applyVoteDeltasToMatchups(ctx context.Context, matchups []models.Matchup) {
	if len(matchups) == 0 {
		return
	}

	// Count first so we can size the IDs slice exactly — avoids the
	// growing-slice realloc cost on hot list endpoints.
	total := 0
	for i := range matchups {
		total += len(matchups[i].Items)
	}
	if total == 0 {
		return
	}

	ids := make([]uint, 0, total)
	for i := range matchups {
		for j := range matchups[i].Items {
			ids = append(ids, matchups[i].Items[j].ID)
		}
	}

	deltas := loadVoteDeltas(ctx, ids)
	if len(deltas) == 0 {
		return
	}
	for i := range matchups {
		items := matchups[i].Items
		for j := range items {
			if d, ok := deltas[items[j].ID]; ok {
				adjusted := int64(items[j].Votes) + d
				if adjusted < 0 {
					adjusted = 0
				}
				items[j].Votes = int(adjusted)
			}
		}
	}
}

// getMatchupCommentsByMatchupIDs batch-fetches all comments for a set of matchup IDs,
// with Author populated, and returns a map of matchup_id → []Comment.
func getMatchupCommentsByMatchupIDs(db sqlx.ExtContext, matchupIDs []uint) map[uint][]models.Comment {
	if len(matchupIDs) == 0 {
		return map[uint][]models.Comment{}
	}
	query, args, err := sqlx.In(
		"SELECT * FROM comments WHERE matchup_id IN (?) ORDER BY created_at DESC",
		matchupIDs,
	)
	if err != nil {
		return map[uint][]models.Comment{}
	}
	var comments []models.Comment
	if err := sqlx.SelectContext(context.Background(), db, &comments, db.Rebind(query), args...); err != nil {
		return map[uint][]models.Comment{}
	}

	// Batch-load comment authors
	userIDSet := make(map[uint]struct{}, len(comments))
	for _, c := range comments {
		userIDSet[c.UserID] = struct{}{}
	}
	userIDs := make([]uint, 0, len(userIDSet))
	for id := range userIDSet {
		userIDs = append(userIDs, id)
	}
	userMap := getUsersByIDs(db, userIDs)
	for i := range comments {
		if u, ok := userMap[comments[i].UserID]; ok {
			comments[i].Author = *u
		}
	}

	result := make(map[uint][]models.Comment, len(matchupIDs))
	for _, c := range comments {
		result[c.MatchupID] = append(result[c.MatchupID], c)
	}
	return result
}
