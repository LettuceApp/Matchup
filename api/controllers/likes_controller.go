package controllers

import (
	"Matchup/api/models"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// LikeMatchup allows a user to like a specific matchup
func (server *Server) LikeMatchup(c *gin.Context) {
	// Get the authenticated user's ID
	tokenID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	uid := tokenID.(uint)

	matchupID := c.Param("id")
	mid, err := strconv.ParseUint(matchupID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid matchup ID"})
		return
	}

	// Check if the matchup exists
	matchup := models.Matchup{}
	err = server.DB.Model(models.Matchup{}).Where("id = ?", uint(mid)).Take(&matchup).Error
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Matchup not found"})
		return
	}

	// Check if the user has already liked the matchup
	existingLike := models.Like{}
	err = server.DB.Where("user_id = ? AND matchup_id = ?", uid, uint(mid)).First(&existingLike).Error
	if err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "You have already liked this matchup"})
		return
	} else if err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error checking existing like"})
		return
	}

	// Create the like
	like := models.Like{
		UserID:    uid,
		MatchupID: uint(mid),
	}
	if err := server.DB.Create(&like).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error liking matchup"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"status": http.StatusCreated, "response": "Matchup liked successfully"})
}

// UnLikeMatchup allows a user to remove their like from a specific matchup
func (server *Server) UnLikeMatchup(c *gin.Context) {
	// Get the authenticated user's ID
	tokenID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	uid := tokenID.(uint)

	matchupID := c.Param("id")
	mid, err := strconv.ParseUint(matchupID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid matchup ID"})
		return
	}

	// Find the like by the user on the specific matchup
	existingLike := models.Like{}
	err = server.DB.Where("user_id = ? AND matchup_id = ?", uid, uint(mid)).First(&existingLike).Error
	if err == gorm.ErrRecordNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "Like not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error finding like"})
		return
	}

	// Delete the like
	if err := server.DB.Delete(&existingLike).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error unliking matchup"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "response": "Matchup unliked successfully"})
}

func (server *Server) GetLikes(c *gin.Context) {
	matchupID := c.Param("id")
	mid, err := strconv.ParseUint(matchupID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid matchup ID"})
		return
	}

	like := models.Like{}
	likes, err := like.GetLikesInfo(server.DB, uint(mid))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No likes found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "response": likes})
}

// GetUserLikes retrieves all likes made by a specific user
func (server *Server) GetUserLikes(c *gin.Context) {
	userID := c.Param("id")
	uid, err := strconv.ParseUint(userID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	likes := []models.Like{}
	err = server.DB.Where("user_id = ?", uint(uid)).Find(&likes).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving user likes"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "response": likes})
}
