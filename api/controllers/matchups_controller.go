package controllers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"Matchup/api/auth"
	"Matchup/api/models"
	"Matchup/api/utils/formaterror"

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

	matchup.AuthorID = uid // the authenticated user is the one creating the matchup

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

// GetMatchups retrieves all matchups along with their items and comments
func (server *Server) GetMatchups(c *gin.Context) {
	var matchups []models.Matchup

	// Use Preload to avoid N+1 query problem
	err := server.DB.Preload("Items").
		Preload("Comments").
		Find(&matchups).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to retrieve matchups"})
		return
	}

	// Convert each matchup to response format
	var response []map[string]interface{}
	for _, matchup := range matchups {
		// Retrieve likes count for each matchup
		var likesCount int64
		server.DB.Model(&models.Like{}).Where("matchup_id = ?", matchup.ID).Count(&likesCount)

		// Retrieve comments for each matchup
		comment := models.Comment{}
		comments, err := comment.GetComments(server.DB, matchup.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving comments"})
			return
		}

		response = append(response, toMatchupResponse(&matchup, *comments, likesCount))
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": response,
	})
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
	tokenID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	uid := tokenID.(uint)

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
	if existingMatchup.AuthorID != uid {
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

// DeleteMatchup allows the author to delete their matchup
// matchups_controller.go
func (server *Server) DeleteMatchup(c *gin.Context) {
	matchupID := c.Param("id")
	mid64, err := strconv.ParseUint(matchupID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid matchup ID"})
		return
	}
	mid := uint(mid64)

	// auth
	tokenID, ok := c.Get("userID")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	uid := tokenID.(uint)

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
	if m.AuthorID != uid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "You are not authorized to delete this matchup"})
		return
	}

	// delete children first, then the parent
	tx := server.DB.Begin()

	if err := tx.Where("matchup_id = ?", mid).Delete(&models.MatchupItem{}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error deleting matchup items"})
		return
	}
	if err := tx.Where("matchup_id = ?", mid).Delete(&models.Comment{}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error deleting comments"})
		return
	}
	if err := tx.Where("matchup_id = ?", mid).Delete(&models.Like{}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error deleting likes"})
		return
	}

	if err := tx.Where("id = ?", mid).Delete(&models.Matchup{}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error deleting matchup"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error finalizing delete"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Matchup deleted"})
}

// GetUserMatchups retrieves all matchups created by a specific user
func (server *Server) GetUserMatchups(c *gin.Context) {
	userID := c.Param("id") // Accept user ID as a path parameter
	uid, err := strconv.ParseUint(userID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var matchups []models.Matchup
	err = server.DB.Preload("Author").
		Preload("Items").
		Preload("Comments").
		Where("author_id = ?", uint(uid)).
		Find(&matchups).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to retrieve matchups"})
		return
	}

	// Convert matchups to response format
	var response []map[string]interface{}
	for _, matchup := range matchups {
		// Retrieve comments for each matchup
		comment := models.Comment{}
		comments, err := comment.GetComments(server.DB, matchup.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving comments"})
			return
		}

		// Retrieve likes count for each matchup
		var likesCount int64
		server.DB.Model(&models.Like{}).Where("matchup_id = ?", matchup.ID).Count(&likesCount)

		// Convert matchup to response format with likes count
		response = append(response, toMatchupResponse(&matchup, *comments, likesCount))
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": response,
	})
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
	tokenID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	uid := tokenID.(uint)

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
	if item.Matchup.AuthorID != uid {
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
	tokenID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	uid := tokenID.(uint)

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
	if matchup.AuthorID != uid {
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
	tokenID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	uid := tokenID.(uint)

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
	if item.Matchup.AuthorID != uid {
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
