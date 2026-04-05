package controllers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"time"

	appdb "Matchup/db"

	"github.com/jmoiron/sqlx"
)

// parseLimit parses a limit string, returning a sane default if invalid.
func parseLimit(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 100 {
		return 20
	}
	return n
}

// removeUserFollowEdges deletes all follow edges involving the given user and
// adjusts follower/following counts on affected users.
func removeUserFollowEdges(db sqlx.ExtContext, userID uint) error {
	// Decrement following_count for users this user follows.
	if _, err := db.ExecContext(context.Background(),
		`UPDATE users SET following_count = GREATEST(following_count - 1, 0)
		 WHERE id = $1`, userID,
	); err != nil {
		return err
	}
	// Decrement followers_count for users that follow this user.
	if _, err := db.ExecContext(context.Background(),
		`UPDATE users SET followers_count = GREATEST(followers_count - 1, 0)
		 WHERE id IN (SELECT follower_id FROM follows WHERE followed_id = $1)`, userID,
	); err != nil {
		return err
	}
	// Decrement following_count for users this user is followed by.
	if _, err := db.ExecContext(context.Background(),
		`UPDATE users SET following_count = GREATEST(following_count - 1, 0)
		 WHERE id IN (SELECT followed_id FROM follows WHERE follower_id = $1)`, userID,
	); err != nil {
		return err
	}
	// Delete all follow edges.
	if _, err := db.ExecContext(context.Background(),
		"DELETE FROM follows WHERE follower_id = $1 OR followed_id = $1", userID,
	); err != nil {
		return err
	}
	return nil
}

// loadViewerRelationships returns two maps: one for users the viewer follows,
// one for users that follow the viewer.
func loadViewerRelationships(db *sqlx.DB, viewerID uint, hasViewer bool, rows []followRow) (followingMap, followedByMap map[uint]bool, err error) {
	followingMap = make(map[uint]bool)
	followedByMap = make(map[uint]bool)
	if !hasViewer || len(rows) == 0 {
		return
	}

	userIDs := make([]uint, len(rows))
	for i, r := range rows {
		userIDs[i] = r.UserID
	}

	// Who the viewer follows among these users.
	q, args, _ := sqlx.In("SELECT followed_id FROM follows WHERE follower_id = ? AND followed_id IN (?)", viewerID, userIDs)
	q = db.Rebind(q)
	var following []uint
	if err = sqlx.SelectContext(context.Background(), db, &following, q, args...); err != nil {
		return
	}
	for _, id := range following {
		followingMap[id] = true
	}

	// Who among these users follows the viewer.
	q2, args2, _ := sqlx.In("SELECT follower_id FROM follows WHERE followed_id = ? AND follower_id IN (?)", viewerID, userIDs)
	q2 = db.Rebind(q2)
	var followedBy []uint
	if err = sqlx.SelectContext(context.Background(), db, &followedBy, q2, args2...); err != nil {
		return
	}
	for _, id := range followedBy {
		followedByMap[id] = true
	}
	return
}

// buildFollowListResponse converts follow rows to DTOs and produces a next-page cursor.
func buildFollowListResponse(rows []followRow, limit int, hasViewer bool, followingMap, followedByMap map[uint]bool) ([]FollowUserDTO, *string) {
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	dtos := make([]FollowUserDTO, len(rows))
	for i, r := range rows {
		avatarPath := appdb.ProcessAvatarPath(r.AvatarPath)
		isFollowing := followingMap[r.UserID]
		isFollowedBy := followedByMap[r.UserID]
		dtos[i] = FollowUserDTO{
			ID:               r.PublicID,
			Username:         r.Username,
			Email:            r.Email,
			AvatarPath:       avatarPath,
			IsAdmin:          r.IsAdmin,
			FollowersCount:   int(r.FollowersCount),
			FollowingCount:   int(r.FollowingCount),
			ViewerFollowing:  hasViewer && isFollowing,
			ViewerFollowedBy: hasViewer && isFollowedBy,
			Mutual:           hasViewer && isFollowing && isFollowedBy,
		}
	}

	var nextCursor *string
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		c := followCursor{ID: last.FollowID, CreatedAt: last.FollowCreatedAt}
		if s, err := encodeFollowCursor(c); err == nil {
			nextCursor = &s
		}
	}
	return dtos, nextCursor
}

// parseFollowCursor decodes a base64-encoded JSON cursor string.
func parseFollowCursor(s string) (*followCursor, error) {
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	var c followCursor
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func encodeFollowCursor(c followCursor) (string, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// MarshalJSON / UnmarshalJSON for followCursor to handle time.Time correctly.
func (c followCursor) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID        uint   `json:"id"`
		CreatedAt string `json:"created_at"`
	}{
		ID:        c.ID,
		CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339Nano),
	})
}

func (c *followCursor) UnmarshalJSON(data []byte) error {
	var v struct {
		ID        uint   `json:"id"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	t, err := time.Parse(time.RFC3339Nano, v.CreatedAt)
	if err != nil {
		return err
	}
	c.ID = v.ID
	c.CreatedAt = t
	return nil
}
