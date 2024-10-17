package controllers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"Matchup/api/auth"
	"Matchup/api/models"
	"Matchup/api/utils/formaterror"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// toMatchupResponse converts a Matchup model to a response-friendly structure
func toMatchupResponse(matchup *models.Matchup, comments []models.Comment) map[string]interface{} {
	// Create the items response structure
	itemResponses := make([]map[string]interface{}, len(matchup.Items))
	for i, item := range matchup.Items {
		itemResponses[i] = map[string]interface{}{
			"id":    item.ID,
			"item":  item.Item,
			"votes": item.Votes,
		}
	}

	// Create the comments response structure
	commentResponses := make([]map[string]interface{}, len(comments))
	for i, comment := range comments {
		commentResponses[i] = map[string]interface{}{
			"id":         comment.ID,
			"user_id":    comment.UserID,
			"username":   comment.User.Username,
			"body":       comment.Body,
			"created_at": comment.CreatedAt,
			"updated_at": comment.UpdatedAt,
		}
	}

	// Return a formatted matchup response without exposing sensitive information
	return map[string]interface{}{
		"id":         matchup.ID,
		"title":      matchup.Title,
		"content":    matchup.Content,
		"author_id":  matchup.AuthorID,
		"items":      itemResponses,
		"comments":   commentResponses,
		"created_at": matchup.CreatedAt,
		"updated_at": matchup.UpdatedAt,
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

	// Prepare response without exposing unnecessary fields (like password)
	matchupResponse := toMatchupResponse(matchupCreated, *comments)

	c.JSON(http.StatusCreated, gin.H{
		"status":   http.StatusCreated,
		"response": matchupResponse,
	})
}

// GetMatchups retrieves all matchups along with their items and comments
func (server *Server) GetMatchups(c *gin.Context) {
	var matchups []models.Matchup

	// Use Preload to avoid N+1 query problem
	err := server.DB.Preload("Author").
		Preload("Items").
		Preload("Comments").
		Find(&matchups).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to retrieve matchups"})
		return
	}

	c.JSON(http.StatusOK, matchups)
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
	err = server.DB.Preload("Author").
		Preload("Items").
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

	c.JSON(http.StatusOK, matchup)
}

// UpdateMatchup allows the author to update their matchup
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

	// Parse JSON input
	var matchup models.Matchup
	if err := c.ShouldBindJSON(&matchup); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update fields
	existingMatchup.Title = matchup.Title
	existingMatchup.Content = matchup.Content
	existingMatchup.UpdatedAt = matchup.UpdatedAt

	// Validate the updated matchup
	existingMatchup.Prepare()
	errorMessages := existingMatchup.Validate()
	if len(errorMessages) > 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"errors": errorMessages})
		return
	}

	// Save the updated matchup
	updatedMatchup, err := existingMatchup.UpdateMatchup(server.DB)
	if err != nil {
		formattedError := formaterror.FormatError(err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"errors": formattedError})
		return
	}

	// Return response with the "response" field
	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": updatedMatchup,
	})
}

// DeleteMatchup allows the author to delete their matchup
func (server *Server) DeleteMatchup(c *gin.Context) {
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
		c.JSON(http.StatusUnauthorized, gin.H{"error": "You are not authorized to delete this matchup"})
		return
	}

	// Delete the matchup (associated data is deleted via ON DELETE CASCADE)
	if _, err := matchup.DeleteMatchup(server.DB, uint(mid)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error deleting matchup"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Matchup deleted"})
}

// GetUserMatchups retrieves all matchups created by a specific user
func (server *Server) GetUserMatchups(c *gin.Context) {
	userID := c.Param("id")
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

	c.JSON(http.StatusOK, matchups)
}

// IncrementMatchupItemVotes increments the vote count for a specific matchup item
func (server *Server) IncrementMatchupItemVotes(c *gin.Context) {
	itemID := c.Param("id")
	iid, err := strconv.ParseUint(itemID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid item ID"})
		return
	}

	// Use atomic increment to avoid race conditions
	err = server.DB.Model(&models.MatchupItem{}).
		Where("id = ?", uint(iid)).
		UpdateColumn("votes", gorm.Expr("votes + ?", 1)).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating votes"})
		return
	}

	// Retrieve the updated item
	var updatedItem models.MatchupItem
	err = server.DB.First(&updatedItem, uint(iid)).Error
	if err != nil {
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
