package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"Matchup/auth"
	"Matchup/cache"
	"Matchup/models"
	"Matchup/utils/formaterror"
	httpctx "Matchup/utils/httpctx"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	defaultMatchupDuration        = 24 * time.Hour
	minMatchupDurationSeconds int = 60
	maxMatchupDurationSeconds int = 86400
)

func normalizeMatchupDurationSeconds(seconds int) int {
	if seconds <= 0 {
		return int(defaultMatchupDuration.Seconds())
	}
	if seconds < minMatchupDurationSeconds {
		return minMatchupDurationSeconds
	}
	if seconds > maxMatchupDurationSeconds {
		return maxMatchupDurationSeconds
	}
	return seconds
}

// toMatchupResponse converts a Matchup model to a response-friendly structure
func toMatchupResponse(matchup *models.Matchup, comments []models.Comment, likesCount int64) map[string]interface{} {

	author := map[string]interface{}{
		"id":       matchup.AuthorID,
		"username": matchup.Author.Username,
	}

	itemResponses := make([]map[string]interface{}, len(matchup.Items))
	for i, item := range matchup.Items {
		itemResponses[i] = map[string]interface{}{
			"id":    item.ID,
			"item":  item.Item,
			"votes": item.Votes,
		}
	}

	commentResponses := make([]map[string]interface{}, len(comments))
	for i, comment := range comments {
		commentResponses[i] = map[string]interface{}{
			"id":         comment.ID,
			"user_id":    comment.UserID,
			"username":   comment.Author.Username,
			"body":       comment.Body,
			"created_at": comment.CreatedAt,
			"updated_at": comment.UpdatedAt,
		}
	}

	return map[string]interface{}{
		"id":               matchup.ID,
		"title":            matchup.Title,
		"content":          matchup.Content,
		"author_id":        matchup.AuthorID,
		"author":           author,
		"items":            itemResponses,
		"comments":         commentResponses,
		"likes_count":      likesCount,
		"created_at":       matchup.CreatedAt,
		"updated_at":       matchup.UpdatedAt,
		"bracket_id":       matchup.BracketID,
		"round":            matchup.Round,
		"seed":             matchup.Seed,
		"status":           matchup.Status,
		"end_mode":         matchup.EndMode,
		"duration_seconds": matchup.DurationSeconds,
		"start_time":       matchup.StartTime,
		"end_time":         matchup.EndTime,
		"winner_item_id":   matchup.WinnerItemID,
	}
}

func toAdminMatchupSummary(matchup *models.Matchup, likesCount int64) map[string]interface{} {
	itemResponses := make([]map[string]interface{}, len(matchup.Items))
	for i, item := range matchup.Items {
		itemResponses[i] = map[string]interface{}{
			"id":    item.ID,
			"item":  item.Item,
			"votes": item.Votes,
		}
	}

	authorUsername := ""
	if matchup.Author.ID != 0 {
		authorUsername = matchup.Author.Username
	}

	return map[string]interface{}{
		"id":              matchup.ID,
		"title":           matchup.Title,
		"content":         matchup.Content,
		"author_id":       matchup.AuthorID,
		"author_username": authorUsername,
		"items":           itemResponses,
		"likes_count":     likesCount,
		"created_at":      matchup.CreatedAt,
		"updated_at":      matchup.UpdatedAt,
	}
}

// CreateMatchup allows authenticated users to create a new matchup
func (server *Server) CreateMatchup(c *gin.Context) {
	errList := map[string]string{}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		errList["Invalid_body"] = "Unable to get request"
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
			"error":  errList,
		})
		return
	}
	matchup := models.Matchup{}

	err = json.Unmarshal(body, &matchup)
	if err != nil {
		errList["Unmarshal_error"] = "Cannot unmarshal body"
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
			"error":  errList,
		})
		return
	}

	uid, err := auth.ExtractTokenID(c.Request)
	if err != nil {
		errList["Unauthorized"] = "Unauthorized"
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": http.StatusUnauthorized,
			"error":  errList,
		})
		return
	}

	user := models.User{}
	err = server.DB.Model(models.User{}).Where("id = ?", uid).Take(&user).Error
	if err != nil {
		errList["Unauthorized"] = "Unauthorized"
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": http.StatusUnauthorized,
			"error":  errList,
		})
		return
	}

	matchup.AuthorID = uid

	if matchup.BracketID == nil && len(matchup.Items) > 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Standalone matchups can only have up to 5 items"})
		return
	}
	if matchup.BracketID != nil && len(matchup.Items) > 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bracket matchups can only have 2 items"})
		return
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
	errorMessages := matchup.Validate()
	if len(errorMessages) > 0 {
		errList = errorMessages
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
			"error":  errList,
		})
		return
	}

	tx := server.DB.Begin()
	matchupCreated, err := matchup.SaveMatchup(tx)
	if err != nil {
		errList := formaterror.FormatError(err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  errList,
		})
		return
	}
	tx.Commit()

	ctx := context.Background()
	_ = cache.DeleteByPrefix(ctx, "matchups:")
	userPrefix := fmt.Sprintf("user:%d:matchups:", uid)
	_ = cache.DeleteByPrefix(ctx, userPrefix)

	comment := models.Comment{}
	comments, err := comment.GetComments(server.DB, matchupCreated.ID)
	if err != nil {
		errList["No_comments"] = "No Comments Found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	var likesCount int64
	server.DB.Model(&models.Like{}).Where("matchup_id = ?", matchupCreated.ID).Count(&likesCount)

	matchupResponse := toMatchupResponse(matchupCreated, *comments, likesCount)

	c.JSON(http.StatusCreated, gin.H{
		"status":   http.StatusCreated,
		"response": matchupResponse,
	})
}

// GetMatchups retrieves paginated matchups along with their items, comments, and likes
func (server *Server) GetMatchups(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "10")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		limit = 10
	}

	ctx := context.Background()
	cacheKey := fmt.Sprintf("matchups:page:%d:limit:%d", page, limit)

	if cached, err := cache.Get(ctx, cacheKey); err == nil && cached != "" {
		c.Data(http.StatusOK, "application/json", []byte(cached))
		return
	}

	offset := (page - 1) * limit

	var total int64
	if err := server.DB.Model(&models.Matchup{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to count matchups"})
		return
	}

	var matchups []models.Matchup
	err = server.DB.Preload("Items").
		Preload("Comments").
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&matchups).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to retrieve matchups"})
		return
	}

	var response []map[string]interface{}
	for i := range matchups {
		matchup := &matchups[i]
		if err := server.finalizeStandaloneMatchupIfExpired(matchup); err != nil {
			log.Printf("auto finalize matchup %d: %v", matchup.ID, err)
		}

		var likesCount int64
		server.DB.Model(&models.Like{}).Where("matchup_id = ?", matchup.ID).Count(&likesCount)

		comment := models.Comment{}
		comments, err := comment.GetComments(server.DB, matchup.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving comments"})
			return
		}

		response = append(response, toMatchupResponse(matchup, *comments, likesCount))
	}

	totalPages := int((total + int64(limit) - 1) / int64(limit))

	respBody := gin.H{
		"status":   http.StatusOK,
		"response": response,
		"pagination": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": totalPages,
		},
	}

	if jsonBytes, err := json.Marshal(respBody); err == nil {
		_ = cache.Set(ctx, cacheKey, jsonBytes, 30*time.Second)
	}

	c.JSON(http.StatusOK, respBody)
}

// GetMatchup retrieves a specific matchup by ID
func (server *Server) GetMatchup(c *gin.Context) {
	matchupID := c.Param("id")
	mid, err := strconv.ParseUint(matchupID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid matchup ID"})
		return
	}

	var matchup models.Matchup
	err = server.DB.Preload("Items").
		Preload("Comments").
		First(&matchup, uint(mid)).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Matchup not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving matchup"})
		}
		return
	}

	if err := dedupeBracketMatchupItems(server.DB, &matchup); err != nil {
		log.Printf("dedupe bracket matchup %d: %v", matchup.ID, err)
	}

	if err := server.finalizeStandaloneMatchupIfExpired(&matchup); err != nil {
		log.Printf("auto finalize matchup %d: %v", matchup.ID, err)
	}

	var likesCount int64
	server.DB.Model(&models.Like{}).Where("matchup_id = ?", matchup.ID).Count(&likesCount)

	comment := models.Comment{}
	comments, err := comment.GetComments(server.DB, matchup.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving comments"})
		return
	}

	matchupResponse := toMatchupResponse(&matchup, *comments, likesCount)

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": matchupResponse,
	})
}

func (server *Server) UpdateMatchup(c *gin.Context) {
	matchupID := c.Param("id")
	mid, err := strconv.ParseUint(matchupID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid matchup ID"})
		return
	}

	requestorID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var existingMatchup models.Matchup
	err = server.DB.First(&existingMatchup, uint(mid)).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Matchup not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving matchup"})
		}
		return
	}
	if existingMatchup.AuthorID != requestorID && !httpctx.IsAdminRequest(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "You are not authorized to update this matchup"})
		return
	}

	var inputData map[string]interface{}
	if err := c.ShouldBindJSON(&inputData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if title, ok := inputData["title"].(string); ok {
		existingMatchup.Title = title
	}

	if content, ok := inputData["content"].(string); ok {
		existingMatchup.Content = content
	}

	existingMatchup.UpdatedAt = time.Now()

	existingMatchup.Prepare()
	errorMessages := existingMatchup.Validate()
	if len(errorMessages) > 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"errors": errorMessages})
		return
	}

	if err := server.DB.Save(&existingMatchup).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"errors": err.Error()})
		return
	}

	var updatedMatchup models.Matchup
	err = server.DB.Preload("Items").Preload("Comments").
		First(&updatedMatchup, existingMatchup.ID).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving updated matchup"})
		return
	}

	comment := models.Comment{}
	comments, err := comment.GetComments(server.DB, updatedMatchup.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving comments"})
		return
	}

	var likesCount int64
	server.DB.Model(&models.Like{}).Where("matchup_id = ?", updatedMatchup.ID).Count(&likesCount)

	matchupResponse := toMatchupResponse(&updatedMatchup, *comments, likesCount)

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": matchupResponse,
	})
}

func (server *Server) deleteMatchupCascade(matchup *models.Matchup) error {
	tx := server.DB.Begin()

	if err := tx.Where("matchup_id = ?", matchup.ID).Delete(&models.MatchupItem{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Where("matchup_id = ?", matchup.ID).Delete(&models.Comment{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Where("matchup_id = ?", matchup.ID).Delete(&models.Like{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Where("matchup_id = ?", matchup.ID).Delete(&models.MatchupVote{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Where("id = ?", matchup.ID).Delete(&models.Matchup{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit().Error; err != nil {
		return err
	}

	ctx := context.Background()
	_ = cache.DeleteByPrefix(ctx, "matchups:")

	userPrefix := fmt.Sprintf("user:%d:matchups:", matchup.AuthorID)
	_ = cache.DeleteByPrefix(ctx, userPrefix)
	invalidateHomeSummaryCache(matchup.AuthorID)
	if matchup.BracketID != nil {
		invalidateBracketSummaryCache(*matchup.BracketID)
	}

	return nil
}

func (server *Server) DeleteMatchup(c *gin.Context) {
	matchupID := c.Param("id")
	mid64, err := strconv.ParseUint(matchupID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid matchup ID"})
		return
	}
	mid := uint(mid64)

	requestorID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var m models.Matchup
	if err := server.DB.First(&m, mid).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Matchup not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving matchup"})
		return
	}
	if m.AuthorID != requestorID && !httpctx.IsAdminRequest(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "You are not authorized to delete this matchup"})
		return
	}

	if err := server.deleteMatchupCascade(&m); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error deleting matchup"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Matchup deleted"})
}

// AdminListMatchups returns a paginated list of matchups for moderation tools.
func (server *Server) AdminListMatchups(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 || limit > 100 {
		limit = 20
	}

	offset := (page - 1) * limit
	search := strings.TrimSpace(c.Query("search"))
	authorParam := strings.TrimSpace(c.Query("author_id"))

	query := server.DB.Model(&models.Matchup{})
	query = query.Where("bracket_id IS NULL")
	if search != "" {
		like := fmt.Sprintf("%%%s%%", search)
		query = query.Where("title ILIKE ? OR content ILIKE ?", like, like)
	}
	if authorParam != "" {
		aid, err := strconv.ParseUint(authorParam, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid author_id"})
			return
		}
		query = query.Where("author_id = ?", uint(aid))
	}

	countQuery := query.Session(&gorm.Session{})
	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to count matchups"})
		return
	}

	var matchups []models.Matchup
	if err := query.
		Preload("Author").
		Preload("Items").
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&matchups).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to fetch matchups"})
		return
	}

	matchupResponses := make([]map[string]interface{}, len(matchups))
	for i := range matchups {
		var likesCount int64
		server.DB.Model(&models.Like{}).Where("matchup_id = ?", matchups[i].ID).Count(&likesCount)
		matchupResponses[i] = toAdminMatchupSummary(&matchups[i], likesCount)
	}

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"response": gin.H{
			"matchups":   matchupResponses,
			"pagination": buildPagination(page, limit, total),
		},
	})
}

func (server *Server) AdminDeleteMatchup(c *gin.Context) {
	matchupID := c.Param("id")
	mid64, err := strconv.ParseUint(matchupID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid matchup ID"})
		return
	}
	mid := uint(mid64)

	var matchup models.Matchup
	if err := server.DB.First(&matchup, mid).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Matchup not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving matchup"})
		return
	}

	if err := server.deleteMatchupCascade(&matchup); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error deleting matchup"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Matchup deleted"})
}

func (server *Server) GetUserMatchups(c *gin.Context) {
	userID := c.Param("id")
	uid, err := strconv.ParseUint(userID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "10")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		limit = 10
	}

	ctx := context.Background()
	cacheKey := fmt.Sprintf("user:%d:matchups:page:%d:limit:%d", uid, page, limit)

	if cached, err := cache.Get(ctx, cacheKey); err == nil && cached != "" {
		c.Data(http.StatusOK, "application/json", []byte(cached))
		return
	}

	offset := (page - 1) * limit

	var total int64
	if err := server.DB.Model(&models.Matchup{}).
		Where("author_id = ?", uint(uid)).
		Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to count user matchups"})
		return
	}

	var matchups []models.Matchup
	err = server.DB.Preload("Author").
		Preload("Items").
		Preload("Comments").
		Where("author_id = ?", uint(uid)).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&matchups).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to retrieve matchups"})
		return
	}

	var response []map[string]interface{}
	for i := range matchups {
		matchup := &matchups[i]
		if err := server.finalizeStandaloneMatchupIfExpired(matchup); err != nil {
			log.Printf("auto finalize matchup %d: %v", matchup.ID, err)
		}

		comment := models.Comment{}
		comments, err := comment.GetComments(server.DB, matchup.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving comments"})
			return
		}

		var likesCount int64
		server.DB.Model(&models.Like{}).Where("matchup_id = ?", matchup.ID).Count(&likesCount)

		response = append(response, toMatchupResponse(matchup, *comments, likesCount))
	}

	totalPages := int((total + int64(limit) - 1) / int64(limit))

	respBody := gin.H{
		"status":   http.StatusOK,
		"response": response,
		"pagination": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": totalPages,
		},
	}

	if jsonBytes, err := json.Marshal(respBody); err == nil {
		_ = cache.Set(ctx, cacheKey, jsonBytes, 30*time.Second)
	}

	c.JSON(http.StatusOK, respBody)
}

func (server *Server) GetUserMatchup(c *gin.Context) {
	userID := c.Param("id")
	matchupID := c.Param("matchupid")

	uid, err := strconv.ParseUint(userID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}
	mid, err := strconv.ParseUint(matchupID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid matchup ID"})
		return
	}

	var matchup models.Matchup
	err = server.DB.Preload("Author").
		Preload("Items").
		Preload("Comments").
		Where("author_id = ? AND id = ?", uint(uid), uint(mid)).
		First(&matchup).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Matchup not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to retrieve matchup"})
		}
		return
	}

	if err := server.finalizeStandaloneMatchupIfExpired(&matchup); err != nil {
		log.Printf("auto finalize matchup %d: %v", matchup.ID, err)
	}

	comment := models.Comment{}
	comments, err := comment.GetComments(server.DB, matchup.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving comments"})
		return
	}

	var likesCount int64
	server.DB.Model(&models.Like{}).Where("matchup_id = ?", matchup.ID).Count(&likesCount)

	response := toMatchupResponse(&matchup, *comments, likesCount)

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": response,
	})
}

func (s *Server) ResolveTieAndAdvance(c *gin.Context) {
	// 🔐 Internal-only (Dagster / trusted frontend flow)
	if c.GetHeader("X-Internal-Key") != os.Getenv("API_SECRET") {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	matchupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid matchup id"})
		return
	}

	var input struct {
		WinnerItemID uint `json:"winner_item_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || input.WinnerItemID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "winner_item_id required"})
		return
	}

	// 🔒 Load matchup with items
	var matchup models.Matchup
	if err := s.DB.
		Preload("Items").
		First(&matchup, uint(matchupID)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "matchup not found"})
		return
	}

	// Must belong to a bracket
	if matchup.BracketID == nil || matchup.Round == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "matchup is not part of a bracket"})
		return
	}

	// 🔒 Load bracket
	var bracket models.Bracket
	if err := s.DB.First(&bracket, *matchup.BracketID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bracket not found"})
		return
	}

	// Bracket must be active and current round
	if bracket.Status != "active" || *matchup.Round != bracket.CurrentRound {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "matchup is not in the current active round",
		})
		return
	}

	// 🔎 Validate winner item belongs to matchup
	valid := false
	for _, item := range matchup.Items {
		if item.ID == input.WinnerItemID {
			valid = true
			break
		}
	}
	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid winner_item_id"})
		return
	}

	// ✅ Resolve matchup
	matchup.WinnerItemID = ptrUint(input.WinnerItemID)
	matchup.Status = matchupStatusCompleted

	if err := s.DB.Save(&matchup).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve matchup"})
		return
	}
	if matchup.BracketID != nil {
		invalidateBracketSummaryCache(*matchup.BracketID)
	}

	// 🚀 Attempt bracket advance (non-fatal if blocked)
	advanced, err := s.advanceBracketInternal(s.DB, &bracket)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"matchup":  matchup,
			"advanced": false,
			"warning":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"matchup":  matchup,
		"advanced": advanced,
	})
}

// IncrementMatchupItemVotes increments the vote count for a specific matchup item
func (server *Server) IncrementMatchupItemVotes(c *gin.Context) {
	itemID := c.Param("id")
	iid, err := strconv.ParseUint(itemID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid item ID"})
		return
	}

	userID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Load item + parent matchup
	var item models.MatchupItem
	if err := server.DB.Preload("Matchup").First(&item, uint(iid)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Matchup item not found"})
		return
	}

	// 🚫 GLOBAL LOCK: completed matchups cannot receive votes
	if item.Matchup.Status == matchupStatusCompleted {
		c.JSON(http.StatusForbidden, gin.H{"error": "Voting is locked for this matchup"})
		return
	}

	if item.Matchup.BracketID == nil && !isMatchupOpenStatus(item.Matchup.Status) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Matchup is not active"})
		return
	}

	if item.Matchup.EndTime != nil && time.Now().After(*item.Matchup.EndTime) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Voting is closed for this matchup"})
		return
	}

	// 🧩 BRACKET-SPECIFIC LOCKS
	if item.Matchup.BracketID != nil {
		var bracket models.Bracket
		if err := server.DB.First(&bracket, *item.Matchup.BracketID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
			return
		}

		if bracket.Status != "active" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Bracket is not active"})
			return
		}

		if item.Matchup.Round == nil || *item.Matchup.Round != bracket.CurrentRound {
			c.JSON(http.StatusForbidden, gin.H{"error": "Voting is locked for this round"})
			return
		}
	}

	var existingVote models.MatchupVote
	err = server.DB.
		Where("user_id = ? AND matchup_id = ?", userID, item.MatchupID).
		First(&existingVote).
		Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error checking existing vote"})
		return
	}

	if err == nil {
		if existingVote.MatchupItemID == item.ID {
			var updatedItem models.MatchupItem
			if err := server.DB.First(&updatedItem, uint(iid)).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving updated item"})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"status":        http.StatusOK,
				"response":      updatedItem,
				"already_voted": true,
			})
			return
		}

		err = server.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&models.MatchupItem{}).
				Where("id = ?", existingVote.MatchupItemID).
				UpdateColumn(
					"votes",
					gorm.Expr("CASE WHEN votes > 0 THEN votes - 1 ELSE 0 END"),
				).
				Error; err != nil {
				return err
			}
			if err := tx.Model(&models.MatchupItem{}).
				Where("id = ?", item.ID).
				UpdateColumn("votes", gorm.Expr("votes + ?", 1)).
				Error; err != nil {
				return err
			}
			return tx.Model(&models.MatchupVote{}).
				Where("id = ?", existingVote.ID).
				Update("matchup_item_id", item.ID).
				Error
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating votes"})
			return
		}
	} else {
		err = server.DB.Transaction(func(tx *gorm.DB) error {
			vote := models.MatchupVote{
				UserID:        userID,
				MatchupID:     item.MatchupID,
				MatchupItemID: item.ID,
			}
			if err := tx.Create(&vote).Error; err != nil {
				return err
			}
			return tx.Model(&models.MatchupItem{}).
				Where("id = ?", item.ID).
				UpdateColumn("votes", gorm.Expr("votes + ?", 1)).
				Error
		})
		if err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				c.JSON(http.StatusConflict, gin.H{"error": "User has already voted for this matchup"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating votes"})
			return
		}
	}

	var updatedItem models.MatchupItem
	if err := server.DB.First(&updatedItem, uint(iid)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving updated item"})
		return
	}
	if item.Matchup.BracketID != nil {
		invalidateBracketSummaryCache(*item.Matchup.BracketID)
	}
	invalidateHomeSummaryCache(item.Matchup.AuthorID)

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": updatedItem,
	})
}

func (server *Server) DeleteMatchupItem(c *gin.Context) {
	itemID := c.Param("id")
	iid, err := strconv.ParseUint(itemID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid item ID"})
		return
	}

	requestorID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var item models.MatchupItem
	err = server.DB.Preload("Matchup").First(&item, uint(iid)).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Matchup item not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving item"})
		}
		return
	}

	if item.Matchup.AuthorID != requestorID && !httpctx.IsAdminRequest(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "You are not authorized to delete this item"})
		return
	}

	if err := server.DB.Delete(&item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error deleting item"})
		return
	}
	if item.Matchup.BracketID != nil {
		invalidateBracketSummaryCache(*item.Matchup.BracketID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Item deleted"})
}

func (server *Server) AddItemToMatchup(c *gin.Context) {
	matchupID := c.Param("matchup_id")
	mid, err := strconv.ParseUint(matchupID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid matchup ID"})
		return
	}

	requestorID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var matchup models.Matchup
	err = server.DB.First(&matchup, uint(mid)).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Matchup not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving matchup"})
		}
		return
	}
	if matchup.AuthorID != requestorID && !httpctx.IsAdminRequest(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "You are not authorized to add items to this matchup"})
		return
	}

	var itemCount int64
	if err := server.DB.Model(&models.MatchupItem{}).Where("matchup_id = ?", matchup.ID).Count(&itemCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to count matchup items"})
		return
	}

	if matchup.BracketID != nil {
		var bracket models.Bracket
		if err := server.DB.First(&bracket, *matchup.BracketID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
			return
		}
		if bracket.Status != "draft" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Cannot add items once a bracket is active"})
			return
		}
		if itemCount >= 2 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Bracket matchups can only have 2 items"})
			return
		}
	} else if itemCount >= 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Standalone matchups can only have up to 5 items"})
		return
	}

	var item models.MatchupItem
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item.MatchupID = uint(mid)

	if err := server.DB.Create(&item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error saving the new item"})
		return
	}
	if matchup.BracketID != nil {
		invalidateBracketSummaryCache(*matchup.BracketID)
	}

	c.JSON(http.StatusCreated, item)
}

func (server *Server) UpdateMatchupItem(c *gin.Context) {
	itemID := c.Param("id")

	id, err := strconv.ParseUint(itemID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid item ID"})
		return
	}

	requestorID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var item models.MatchupItem
	if err := server.DB.Preload("Matchup").First(&item, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Matchup item not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to retrieve matchup item"})
		}
		return
	}

	if item.Matchup.AuthorID != requestorID && !httpctx.IsAdminRequest(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "You are not authorized to update this item"})
		return
	}

	// Bracket matchup items can only be edited while the bracket is draft
	if item.Matchup.BracketID != nil {
		var bracket models.Bracket
		if err := server.DB.First(&bracket, *item.Matchup.BracketID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
			return
		}
		if bracket.Status != "draft" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Bracket matchup items cannot be edited once active"})
			return
		}
	}

	// Completed matchups cannot be edited
	if item.Matchup.Status == matchupStatusCompleted {
		c.JSON(http.StatusForbidden, gin.H{"error": "This matchup is completed and cannot be edited"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Unable to read request body"})
		return
	}

	log.Printf("Request Body: %s", string(body))

	var updatedData struct {
		Item string `json:"item"`
	}
	if err := json.Unmarshal(body, &updatedData); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Invalid request body"})
		return
	}

	log.Printf("Parsed Data: %+v", updatedData)

	if item.Matchup.BracketID != nil {
		seed, ok := parseSeedValue(item.Item)
		if !ok {
			seed, ok = parseSeedValue(updatedData.Item)
		}
		if ok {
			item.Item = formatSeedLabel(seed, updatedData.Item)
		} else {
			item.Item = updatedData.Item
		}
	} else {
		item.Item = updatedData.Item
	}

	if err := server.DB.Save(&item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to update matchup item"})
		return
	}
	if item.Matchup.BracketID != nil {
		invalidateBracketSummaryCache(*item.Matchup.BracketID)
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "response": item})
}

func (s *Server) OverrideMatchupWinner(c *gin.Context) {
	matchupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid matchup ID"})
		return
	}

	userID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var matchup models.Matchup
	if err := s.DB.Preload("Items").First(&matchup, uint(matchupID)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Matchup not found"})
		return
	}

	if matchup.AuthorID != userID && !httpctx.IsAdminRequest(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authorized"})
		return
	}

	if matchup.Status == matchupStatusCompleted && matchup.BracketID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Matchup is completed and cannot be overridden"})
		return
	}

	// ------------------------------
	// BRACKET VALIDATION (if exists)
	// ------------------------------
	if matchup.BracketID != nil {
		var b models.Bracket
		if err := s.DB.First(&b, *matchup.BracketID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
			return
		}

		if b.Status != "active" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot override winner for this bracket at this time"})
			return
		}

		if matchup.Round == nil || *matchup.Round != b.CurrentRound {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot override winner outside the current round"})
			return
		}

	}

	// ------------------------------
	// INPUT
	// ------------------------------
	var input struct {
		WinnerItemID uint `json:"winner_item_id"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	valid := false
	for _, item := range matchup.Items {
		if item.ID == input.WinnerItemID {
			valid = true
			break
		}
	}
	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid winner item"})
		return
	}

	// ------------------------------
	// SAVE WINNER
	// ------------------------------
	matchup.WinnerItemID = &input.WinnerItemID
	matchup.Status = matchupStatusCompleted
	matchup.UpdatedAt = time.Now()

	if err := s.DB.Save(&matchup).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to override winner"})
		return
	}
	if matchup.BracketID != nil {
		invalidateBracketSummaryCache(*matchup.BracketID)
	}

	// ------------------------------
	// RESPONSE
	// ------------------------------
	c.JSON(http.StatusOK, gin.H{
		"message":        "Winner selected",
		"winner_item_id": input.WinnerItemID,
	})
}

// CompleteMatchup finalizes a matchup (Ready Up). Once completed, it is immutable and voting is locked.
// Works for both bracket and non-bracket matchups.
func (server *Server) CompleteMatchup(c *gin.Context) {
	matchupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid matchup ID"})
		return
	}

	userID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var matchup models.Matchup
	if err := server.DB.Preload("Items").First(&matchup, uint(matchupID)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Matchup not found"})
		return
	}

	if matchup.AuthorID != userID && !httpctx.IsAdminRequest(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authorized"})
		return
	}

	var bracket models.Bracket
	if matchup.BracketID != nil {
		if err := server.DB.First(&bracket, *matchup.BracketID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
			return
		}

		if bracket.Status != "active" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot modify matchup for an inactive bracket"})
			return
		}

		if matchup.Round == nil || *matchup.Round != bracket.CurrentRound {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot modify matchup outside the current bracket round"})
			return
		}
	}

	// Toggle off if already ready
	if matchup.Status == matchupStatusCompleted {
		matchup.Status = matchupStatusActive
		matchup.UpdatedAt = time.Now()

		if err := server.DB.Save(&matchup).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reopen matchup"})
			return
		}
		if matchup.BracketID != nil {
			invalidateBracketSummaryCache(*matchup.BracketID)
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Matchup reopened",
			"status":  matchup.Status,
		})
		return
	}

	winnerID, err := determineMatchupWinner(&matchup)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	matchup.Status = matchupStatusCompleted
	matchup.WinnerItemID = &winnerID
	matchup.UpdatedAt = time.Now()

	if err := server.DB.Save(&matchup).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to complete matchup"})
		return
	}
	if matchup.BracketID != nil {
		invalidateBracketSummaryCache(*matchup.BracketID)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "Matchup completed",
		"status":         matchup.Status,
		"winner_item_id": winnerID,
	})
}

func (s *Server) ReadyUpMatchup(c *gin.Context) {
	matchupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid matchup ID"})
		return
	}

	userID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var matchup models.Matchup
	if err := s.DB.Preload("Items").First(&matchup, uint(matchupID)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Matchup not found"})
		return
	}

	if matchup.AuthorID != userID && !httpctx.IsAdminRequest(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authorized"})
		return
	}

	if matchup.WinnerItemID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Select a winner first"})
		return
	}

	matchup.Status = matchupStatusCompleted

	if err := s.DB.Save(&matchup).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to ready matchup"})
		return
	}
	if matchup.BracketID != nil {
		invalidateBracketSummaryCache(*matchup.BracketID)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Matchup ready",
		"status":  matchup.Status,
	})
}

func determineMatchupWinner(matchup *models.Matchup) (uint, error) {
	if len(matchup.Items) == 0 {
		return 0, errors.New("matchup has no contenders")
	}

	if matchup.WinnerItemID != nil {
		for _, it := range matchup.Items {
			if it.ID == *matchup.WinnerItemID {
				return it.ID, nil
			}
		}
		return 0, errors.New("selected winner does not belong to this matchup")
	}

	var winnerID uint
	maxVotes := matchup.Items[0].Votes
	winnerID = matchup.Items[0].ID
	topCount := 1

	for i := 1; i < len(matchup.Items); i++ {
		item := matchup.Items[i]
		if item.Votes > maxVotes {
			maxVotes = item.Votes
			winnerID = item.ID
			topCount = 1
		} else if item.Votes == maxVotes {
			topCount++
		}
	}

	if topCount > 1 {
		return 0, errors.New("matchup is tied. Select a winner before readying up")
	}

	return winnerID, nil
}

func determineMatchupWinnerByVotes(matchup *models.Matchup) (uint, error) {
	if len(matchup.Items) == 0 {
		return 0, errors.New("matchup has no contenders")
	}

	winnerID := matchup.Items[0].ID
	maxVotes := matchup.Items[0].Votes

	for _, item := range matchup.Items[1:] {
		if item.Votes > maxVotes {
			maxVotes = item.Votes
			winnerID = item.ID
		}
	}

	return winnerID, nil
}

func parseSeedValue(label string) (int, bool) {
	seed := 0
	found := false

	for _, r := range label {
		if r >= '0' && r <= '9' {
			seed = seed*10 + int(r-'0')
			found = true
		} else if found {
			break
		}
	}

	return seed, found
}

func determineMatchupWinnerByVotesOrSeed(matchup *models.Matchup) (uint, error) {
	if len(matchup.Items) == 0 {
		return 0, errors.New("matchup has no contenders")
	}

	maxVotes := matchup.Items[0].Votes
	tied := []models.MatchupItem{matchup.Items[0]}

	for _, item := range matchup.Items[1:] {
		if item.Votes > maxVotes {
			maxVotes = item.Votes
			tied = []models.MatchupItem{item}
			continue
		}
		if item.Votes == maxVotes {
			tied = append(tied, item)
		}
	}

	if len(tied) == 1 {
		return tied[0].ID, nil
	}

	bestSeed := 0
	bestID := uint(0)
	for _, item := range tied {
		seed, ok := parseSeedValue(item.Item)
		if !ok {
			continue
		}
		if bestID == 0 || seed < bestSeed {
			bestSeed = seed
			bestID = item.ID
		}
	}

	if bestID != 0 {
		return bestID, nil
	}

	return 0, errors.New("matchup is tied")
}

func dedupeBracketMatchupItems(tx *gorm.DB, matchup *models.Matchup) error {
	if matchup.BracketID == nil || len(matchup.Items) <= 2 {
		return nil
	}

	type bucket struct {
		keep      *models.MatchupItem
		votes     int
		deleteIDs []uint
	}

	buckets := make(map[string]*bucket)
	for i := range matchup.Items {
		item := &matchup.Items[i]
		key := strings.TrimSpace(item.Item)
		if key == "" {
			key = fmt.Sprintf("id:%d", item.ID)
		}

		entry, ok := buckets[key]
		if !ok {
			buckets[key] = &bucket{keep: item, votes: item.Votes}
			continue
		}

		entry.votes += item.Votes
		if matchup.WinnerItemID != nil && item.ID == *matchup.WinnerItemID {
			entry.deleteIDs = append(entry.deleteIDs, entry.keep.ID)
			entry.keep = item
		} else {
			entry.deleteIDs = append(entry.deleteIDs, item.ID)
		}
	}

	if len(buckets) > 2 {
		return fmt.Errorf("matchup %d has too many items", matchup.ID)
	}

	for _, entry := range buckets {
		if err := tx.Model(&models.MatchupItem{}).
			Where("id = ?", entry.keep.ID).
			Update("votes", entry.votes).Error; err != nil {
			return err
		}
	}

	var deleteIDs []uint
	for _, entry := range buckets {
		deleteIDs = append(deleteIDs, entry.deleteIDs...)
	}

	if len(deleteIDs) > 0 {
		if err := tx.Where("id IN ?", deleteIDs).
			Delete(&models.MatchupItem{}).Error; err != nil {
			return err
		}
	}

	return tx.Where("matchup_id = ?", matchup.ID).
		Order("id ASC").
		Find(&matchup.Items).Error
}

func (server *Server) finalizeStandaloneMatchupIfExpired(matchup *models.Matchup) error {
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
	matchup.UpdatedAt = time.Now()

	return server.DB.Save(matchup).Error
}

func (s *Server) ActivateMatchup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid matchup id"})
		return
	}

	userID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var matchup models.Matchup
	if err := s.DB.First(&matchup, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "matchup not found"})
		return
	}

	if matchup.AuthorID != userID && !httpctx.IsAdminRequest(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	if matchup.BracketID != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bracket matchups cannot be manually activated"})
		return
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

	if err := s.DB.Save(&matchup).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to activate matchup"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"response": matchup})
}
