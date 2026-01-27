package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"Matchup/models"
	httpctx "Matchup/utils/httpctx"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// FollowUser godoc
// @Summary      Follow a user
// @Description  Follow another user as the authenticated user
// @Tags         follows
// @Produce      json
// @Param        id   path      string  true  "User ID to follow"
// @Success      200  {object}  SimpleMessageResponse
// @Success      201  {object}  SimpleMessageResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /users/{id}/follow [post]
// @Security     BearerAuth
func (server *Server) FollowUser(c *gin.Context) {
	requestorID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	target, err := resolveUserByIdentifier(server.DB, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	targetID := target.ID
	if requestorID == targetID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot follow yourself"})
		return
	}

	created := false
	err = server.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Select("id").First(&models.User{}, targetID).Error; err != nil {
			return err
		}

		follow := models.Follow{
			FollowerID: requestorID,
			FollowedID: targetID,
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&follow)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		created = true

		if err := tx.Model(&models.User{}).
			Where("id = ?", requestorID).
			UpdateColumn("following_count", gorm.Expr("following_count + 1")).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.User{}).
			Where("id = ?", targetID).
			UpdateColumn("followers_count", gorm.Expr("followers_count + 1")).Error; err != nil {
			return err
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error following user"})
		return
	}

	status := http.StatusOK
	message := "Already following user"
	if created {
		status = http.StatusCreated
		message = "User followed successfully"
	}
	c.JSON(status, gin.H{"status": status, "response": message})
}

// UnfollowUser godoc
// @Summary      Unfollow a user
// @Description  Unfollow another user as the authenticated user
// @Tags         follows
// @Produce      json
// @Param        id   path      string  true  "User ID to unfollow"
// @Success      200  {object}  SimpleMessageResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /users/{id}/follow [delete]
// @Security     BearerAuth
func (server *Server) UnfollowUser(c *gin.Context) {
	requestorID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	target, err := resolveUserByIdentifier(server.DB, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	targetID := target.ID
	if requestorID == targetID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot unfollow yourself"})
		return
	}

	err = server.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Select("id").First(&models.User{}, targetID).Error; err != nil {
			return err
		}

		result := tx.Where("follower_id = ? AND followed_id = ?", requestorID, targetID).
			Delete(&models.Follow{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}

		if err := tx.Model(&models.User{}).
			Where("id = ?", requestorID).
			UpdateColumn("following_count", gorm.Expr("GREATEST(following_count - 1, 0)")).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.User{}).
			Where("id = ?", targetID).
			UpdateColumn("followers_count", gorm.Expr("GREATEST(followers_count - 1, 0)")).Error; err != nil {
			return err
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error unfollowing user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "response": "User unfollowed successfully"})
}

// GetFollowers godoc
// @Summary      List followers
// @Description  List followers for a user (cursor-based pagination)
// @Tags         follows
// @Produce      json
// @Param        id      path      string     true   "User ID"
// @Param        limit   query  int     false  "Max results (default 20, max 100)"
// @Param        cursor  query  string  false  "Pagination cursor"
// @Success      200  {object}  FollowListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /users/{id}/followers [get]
func (server *Server) GetFollowers(c *gin.Context) {
	target, err := resolveUserByIdentifier(server.DB, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if err := ensureUserExists(server.DB, target.ID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error loading user"})
		return
	}

	limit := parseLimit(c.DefaultQuery("limit", "20"))
	cursor, err := parseFollowCursor(c.Query("cursor"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cursor"})
		return
	}

	rows, err := server.fetchFollowRows(
		"follows.followed_id = ?",
		[]interface{}{target.ID},
		"users.id = follows.follower_id",
		limit,
		cursor,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching followers"})
		return
	}

	viewerID, hasViewer := optionalViewerID(c)
	followingMap, followedByMap, err := loadViewerRelationships(server.DB, viewerID, hasViewer, rows)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error loading viewer relationships"})
		return
	}
	response, nextCursor := buildFollowListResponse(rows, limit, hasViewer, followingMap, followedByMap)
	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"response": gin.H{
			"users":       response,
			"next_cursor": nextCursor,
		},
	})
}

// GetFollowing godoc
// @Summary      List following
// @Description  List users a user is following (cursor-based pagination)
// @Tags         follows
// @Produce      json
// @Param        id      path      string     true   "User ID"
// @Param        limit   query  int     false  "Max results (default 20, max 100)"
// @Param        cursor  query  string  false  "Pagination cursor"
// @Success      200  {object}  FollowListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /users/{id}/following [get]
func (server *Server) GetFollowing(c *gin.Context) {
	target, err := resolveUserByIdentifier(server.DB, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if err := ensureUserExists(server.DB, target.ID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error loading user"})
		return
	}

	limit := parseLimit(c.DefaultQuery("limit", "20"))
	cursor, err := parseFollowCursor(c.Query("cursor"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cursor"})
		return
	}

	rows, err := server.fetchFollowRows(
		"follows.follower_id = ?",
		[]interface{}{target.ID},
		"users.id = follows.followed_id",
		limit,
		cursor,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching following"})
		return
	}

	viewerID, hasViewer := optionalViewerID(c)
	followingMap, followedByMap, err := loadViewerRelationships(server.DB, viewerID, hasViewer, rows)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error loading viewer relationships"})
		return
	}
	response, nextCursor := buildFollowListResponse(rows, limit, hasViewer, followingMap, followedByMap)
	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"response": gin.H{
			"users":       response,
			"next_cursor": nextCursor,
		},
	})
}

// GetRelationship godoc
// @Summary      Get relationship state
// @Description  Get relationship flags between the authenticated user and a target user
// @Tags         follows
// @Produce      json
// @Param        id   path      string  true  "Target User ID"
// @Success      200  {object}  RelationshipResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /users/{id}/relationship [get]
// @Security     BearerAuth
func (server *Server) GetRelationship(c *gin.Context) {
	requestorID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	target, err := resolveUserByIdentifier(server.DB, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if requestorID == target.ID {
		c.JSON(http.StatusOK, gin.H{
			"following":   false,
			"followed_by": false,
			"mutual":      false,
		})
		return
	}

	var rel struct {
		Following  bool `json:"following"`
		FollowedBy bool `json:"followed_by"`
	}
	if err := server.DB.Raw(
		`SELECT
			EXISTS(SELECT 1 FROM follows WHERE follower_id = ? AND followed_id = ?) AS following,
			EXISTS(SELECT 1 FROM follows WHERE follower_id = ? AND followed_id = ?) AS followed_by`,
		requestorID, target.ID, target.ID, requestorID,
	).Scan(&rel).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error checking relationship"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"following":   rel.Following,
		"followed_by": rel.FollowedBy,
		"mutual":      rel.Following && rel.FollowedBy,
	})
}

func removeUserFollowEdges(tx *gorm.DB, userID uint) error {
	if err := tx.Exec(
		"UPDATE users SET followers_count = GREATEST(followers_count - 1, 0) WHERE id IN (SELECT followed_id FROM follows WHERE follower_id = ?)",
		userID,
	).Error; err != nil {
		return err
	}
	if err := tx.Exec(
		"UPDATE users SET following_count = GREATEST(following_count - 1, 0) WHERE id IN (SELECT follower_id FROM follows WHERE followed_id = ?)",
		userID,
	).Error; err != nil {
		return err
	}
	if err := tx.Where("follower_id = ? OR followed_id = ?", userID, userID).
		Delete(&models.Follow{}).Error; err != nil {
		return err
	}
	return nil
}

func loadViewerRelationships(db *gorm.DB, viewerID uint, hasViewer bool, rows []followRow) (map[uint]bool, map[uint]bool, error) {
	followingMap := make(map[uint]bool)
	followedByMap := make(map[uint]bool)
	if !hasViewer || len(rows) == 0 {
		return followingMap, followedByMap, nil
	}

	ids := make([]uint, len(rows))
	for i := range rows {
		ids[i] = rows[i].User.ID
	}

	var followingIDs []uint
	if err := db.Table("follows").
		Select("followed_id").
		Where("follower_id = ? AND followed_id IN ?", viewerID, ids).
		Scan(&followingIDs).Error; err != nil {
		return nil, nil, err
	}
	for _, id := range followingIDs {
		followingMap[id] = true
	}

	var followedByIDs []uint
	if err := db.Table("follows").
		Select("follower_id").
		Where("followed_id = ? AND follower_id IN ?", viewerID, ids).
		Scan(&followedByIDs).Error; err != nil {
		return nil, nil, err
	}
	for _, id := range followedByIDs {
		followedByMap[id] = true
	}

	return followingMap, followedByMap, nil
}

type followRow struct {
	models.User
	FollowID        uint      `gorm:"column:follow_id"`
	FollowCreatedAt time.Time `gorm:"column:follow_created_at"`
}

type followCursor struct {
	CreatedAt time.Time
	ID        uint
}

func parseFollowCursor(value string) (*followCursor, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid cursor")
	}
	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return nil, err
	}
	return &followCursor{CreatedAt: time.Unix(0, nanos).UTC(), ID: uint(id)}, nil
}

func formatFollowCursor(row followRow) string {
	return fmt.Sprintf("%d:%d", row.FollowCreatedAt.UnixNano(), row.FollowID)
}

func parseLimit(value string) int {
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func ensureUserExists(db *gorm.DB, userID uint) error {
	var user models.User
	return db.Select("id").First(&user, userID).Error
}

func (server *Server) fetchFollowRows(whereClause string, whereArgs []interface{}, joinClause string, limit int, cursor *followCursor) ([]followRow, error) {
	query := server.DB.Table("follows").
		Select("follows.id as follow_id, follows.created_at as follow_created_at, users.*").
		Joins("JOIN users ON "+joinClause).
		Where(whereClause, whereArgs...)

	if cursor != nil {
		query = query.Where(
			"(follows.created_at < ?) OR (follows.created_at = ? AND follows.id < ?)",
			cursor.CreatedAt, cursor.CreatedAt, cursor.ID,
		)
	}

	var rows []followRow
	if err := query.Order("follows.created_at DESC, follows.id DESC").
		Limit(limit + 1).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func buildFollowListResponse(rows []followRow, limit int, hasViewer bool, followingMap map[uint]bool, followedByMap map[uint]bool) ([]FollowUserDTO, *string) {
	if len(rows) == 0 {
		return []FollowUserDTO{}, nil
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	users := make([]FollowUserDTO, len(rows))
	for i := range rows {
		user := rows[i].User
		_ = user.AfterFind(nil)
		payload := FollowUserDTO{
			UserDTO: userToResponse(&user),
		}
		if hasViewer {
			following := followingMap[user.ID]
			followedBy := followedByMap[user.ID]
			payload.ViewerFollowing = following
			payload.ViewerFollowedBy = followedBy
			payload.Mutual = following && followedBy
		}
		users[i] = payload
	}

	var nextCursor *string
	if hasMore {
		cursor := formatFollowCursor(rows[len(rows)-1])
		nextCursor = &cursor
	}
	return users, nextCursor
}
