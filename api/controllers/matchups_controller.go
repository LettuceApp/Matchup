package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"Matchup/api/auth"
	"Matchup/api/cache"
	"Matchup/api/models"
	"Matchup/api/utils/formaterror"
	httpctx "Matchup/api/utils/httpctx"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// toMatchupResponse converts a Matchup model to a response-friendly structure
func toMatchupResponse(matchup *models.Matchup, comments []models.Comment, likesCount int64) map[string]interface{} {
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
			"username":   comment.Author.Username, // Access the username through Author
			"body":       comment.Body,
			"created_at": comment.CreatedAt,
			"updated_at": comment.UpdatedAt,
		}
	}

	return map[string]interface{}{
		"id":          matchup.ID,
		"title":       matchup.Title,
		"content":     matchup.Content,
		"author_id":   matchup.AuthorID,
		"items":       itemResponses,
		"comments":    commentResponses,
		"likes_count": likesCount,
		"created_at":  matchup.CreatedAt,
		"updated_at":  matchup.UpdatedAt,
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
	// Clear previous error if any
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

	// Check if the user exists
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

	// the authenticated user is the one creating the matchup
	matchup.AuthorID = uid

	matchup.Prepare()
	errorMessages := matchup.Validate()
	if len(errorMessages) > 0 {
		errList = errorMessages
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
			"error":  errList,
		})
		return
	}

	// Create the matchup
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

	// 🔥 NEW: Invalidate cached matchup lists in Redis
	ctx := context.Background()
	// Clear all general matchup list pages
	_ = cache.DeleteByPrefix(ctx, "matchups:")
	// Clear this user's matchup list pages
	userPrefix := fmt.Sprintf("user:%d:matchups:", uid)
	_ = cache.DeleteByPrefix(ctx, userPrefix)

	// Retrieve the comments for the created matchup
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

	// Retrieve likes count for the created matchup (initially zero)
	var likesCount int64
	server.DB.Model(&models.Like{}).Where("matchup_id = ?", matchupCreated.ID).Count(&likesCount)

	// Prepare response without exposing unnecessary fields (like password)
	matchupResponse := toMatchupResponse(matchupCreated, *comments, likesCount)

	c.JSON(http.StatusCreated, gin.H{
		"status":   http.StatusCreated,
		"response": matchupResponse,
	})
}

// GetMatchups retrieves paginated matchups along with their items, comments, and likes
func (server *Server) GetMatchups(c *gin.Context) {
	// Parse pagination query params (your existing logic)
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

	// 1) Try Redis cache
	if cached, err := cache.Get(ctx, cacheKey); err == nil && cached != "" {
		// Serve raw JSON from cache
		c.Data(http.StatusOK, "application/json", []byte(cached))
		return
	}

	// 2) Fallback to DB + existing logic

	offset := (page - 1) * limit

	// Count total matchups
	var total int64
	if err := server.DB.Model(&models.Matchup{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to count matchups"})
		return
	}

	// Fetch paginated matchups
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

	// Build response (same as before)
	var response []map[string]interface{}
	for _, matchup := range matchups {
		var likesCount int64
		server.DB.Model(&models.Like{}).Where("matchup_id = ?", matchup.ID).Count(&likesCount)

		comment := models.Comment{}
		comments, err := comment.GetComments(server.DB, matchup.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving comments"})
			return
		}

		response = append(response, toMatchupResponse(&matchup, *comments, likesCount))
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

	// 3) Store JSON in Redis with a short TTL
	if jsonBytes, err := json.Marshal(respBody); err == nil {
		_ = cache.Set(ctx, cacheKey, jsonBytes, 30*time.Second)
	}

	// 4) Return to client
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

	// Retrieve likes count for the matchup
	var likesCount int64
	server.DB.Model(&models.Like{}).Where("matchup_id = ?", matchup.ID).Count(&likesCount)

	// Retrieve comments for the matchup
	comment := models.Comment{}
	comments, err := comment.GetComments(server.DB, matchup.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving comments"})
		return
	}

	// Prepare response
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

	// Get the authenticated user's ID
	requestorID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Check if the matchup exists and belongs to the user
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

	// Parse JSON input into a map
	var inputData map[string]interface{}
	if err := c.ShouldBindJSON(&inputData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update fields based on input
	if title, ok := inputData["title"].(string); ok {
		existingMatchup.Title = title
	}

	if content, ok := inputData["content"].(string); ok {
		existingMatchup.Content = content
	}

	// Update the UpdatedAt timestamp
	existingMatchup.UpdatedAt = time.Now()

	// Validate the updated matchup
	existingMatchup.Prepare()
	errorMessages := existingMatchup.Validate()
	if len(errorMessages) > 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"errors": errorMessages})
		return
	}

	// Save the updated matchup
	if err := server.DB.Save(&existingMatchup).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"errors": err.Error()})
		return
	}

	// Retrieve updated matchup with items and comments
	var updatedMatchup models.Matchup
	err = server.DB.Preload("Items").Preload("Comments").
		First(&updatedMatchup, existingMatchup.ID).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving updated matchup"})
		return
	}

	// Retrieve the comments for the updated matchup
	comment := models.Comment{}
	comments, err := comment.GetComments(server.DB, updatedMatchup.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving comments"})
		return
	}

	// Retrieve likes count for the updated matchup
	var likesCount int64
	server.DB.Model(&models.Like{}).Where("matchup_id = ?", updatedMatchup.ID).Count(&likesCount)

	// Prepare response with likes count
	matchupResponse := toMatchupResponse(&updatedMatchup, *comments, likesCount)

	// Return response
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

	return nil
}

// DeleteMatchup allows the author to delete their matchup
func (server *Server) DeleteMatchup(c *gin.Context) {
	matchupID := c.Param("id")
	mid64, err := strconv.ParseUint(matchupID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid matchup ID"})
		return
	}
	mid := uint(mid64)

	// auth
	requestorID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// load & ownership
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

	// delete children first, then the parent
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

// AdminDeleteMatchup allows admins to delete any matchup regardless of owner.
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

// GetUserMatchups retrieves paginated matchups created by a specific user
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

	// Try Redis cache
	if cached, err := cache.Get(ctx, cacheKey); err == nil && cached != "" {
		c.Data(http.StatusOK, "application/json", []byte(cached))
		return
	}

	offset := (page - 1) * limit

	// Count total matchups for this user
	var total int64
	if err := server.DB.Model(&models.Matchup{}).
		Where("author_id = ?", uint(uid)).
		Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to count user matchups"})
		return
	}

	// Fetch paginated matchups
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
	for _, matchup := range matchups {
		comment := models.Comment{}
		comments, err := comment.GetComments(server.DB, matchup.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving comments"})
			return
		}

		var likesCount int64
		server.DB.Model(&models.Like{}).Where("matchup_id = ?", matchup.ID).Count(&likesCount)

		response = append(response, toMatchupResponse(&matchup, *comments, likesCount))
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

// GetUserMatchup retrieves a specific matchup created by a specific user
func (server *Server) GetUserMatchup(c *gin.Context) {
	userID := c.Param("id")           // Accept user ID as a path parameter
	matchupID := c.Param("matchupid") // Accept matchup ID as a path parameter

	// Parse user ID and matchup ID
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

	// Find the specific matchup for the given user
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

	// Retrieve comments for the matchup
	comment := models.Comment{}
	comments, err := comment.GetComments(server.DB, matchup.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving comments"})
		return
	}

	// Retrieve likes count for the matchup
	var likesCount int64
	server.DB.Model(&models.Like{}).Where("matchup_id = ?", matchup.ID).Count(&likesCount)

	// Convert matchup to response format with likes count
	response := toMatchupResponse(&matchup, *comments, likesCount)

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": response,
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

	// Perform the increment using the model method
	item := models.MatchupItem{ID: uint(iid)}
	if err := item.IncrementVotes(server.DB); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating votes"})
		return
	}

	// Retrieve the updated item to include in the response
	var updatedItem models.MatchupItem
	if err := server.DB.First(&updatedItem, uint(iid)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving updated item"})
		return
	}

	// Return response with the "response" field
	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": updatedItem,
	})
}

// DeleteMatchupItem deletes a specific item from a matchup
func (server *Server) DeleteMatchupItem(c *gin.Context) {
	itemID := c.Param("id")
	iid, err := strconv.ParseUint(itemID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid item ID"})
		return
	}

	// Get the authenticated user's ID
	requestorID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Fetch the item and its parent matchup
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

	// Check if the authenticated user is the author of the matchup
	if item.Matchup.AuthorID != requestorID && !httpctx.IsAdminRequest(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "You are not authorized to delete this item"})
		return
	}

	// Delete the item
	if err := server.DB.Delete(&item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error deleting item"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Item deleted"})
}

// AddItemToMatchup adds a new item to an existing matchup
func (server *Server) AddItemToMatchup(c *gin.Context) {
	matchupID := c.Param("matchup_id")
	mid, err := strconv.ParseUint(matchupID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid matchup ID"})
		return
	}

	// Get the authenticated user's ID
	requestorID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Check if the matchup exists and belongs to the user
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

	// Parse JSON input for the new item
	var item models.MatchupItem
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set the foreign key relationship
	item.MatchupID = uint(mid)

	// Save the new item
	if err := server.DB.Create(&item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error saving the new item"})
		return
	}

	c.JSON(http.StatusCreated, item)
}

// UpdateMatchupItem updates the name of an item in a specific matchup
func (server *Server) UpdateMatchupItem(c *gin.Context) {
	itemID := c.Param("id") // Get item ID from path parameter

	// Parse the item ID to uint
	id, err := strconv.ParseUint(itemID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid item ID"})
		return
	}

	// Get the authenticated user's ID
	requestorID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Find the matchup item by ID
	var item models.MatchupItem
	if err := server.DB.Preload("Matchup").First(&item, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Matchup item not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to retrieve matchup item"})
		}
		return
	}

	// Ensure the authenticated user is the owner of the matchup
	if item.Matchup.AuthorID != requestorID && !httpctx.IsAdminRequest(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "You are not authorized to update this item"})
		return
	}

	// Read the request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Unable to read request body"})
		return
	}

	// Log the request body for debugging
	log.Printf("Request Body: %s", string(body))

	// Parse the new name from the request body
	var updatedData struct {
		Item string `json:"item"`
	}
	if err := json.Unmarshal(body, &updatedData); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Invalid request body"})
		return
	}

	// Log the parsed data for debugging
	log.Printf("Parsed Data: %+v", updatedData)

	// Update only the item (name) field
	item.Item = updatedData.Item

	// Save the updated item to the database
	if err := server.DB.Save(&item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to update matchup item"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "response": item})
}
