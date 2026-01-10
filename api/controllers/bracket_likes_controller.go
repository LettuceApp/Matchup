package controllers

import (
	"Matchup/models"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// LikeBracket allows a user to like a specific bracket
func (server *Server) LikeBracket(c *gin.Context) {
	tokenID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	uid := tokenID.(uint)

	bracketID := c.Param("id")
	bid, err := strconv.ParseUint(bracketID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bracket ID"})
		return
	}

	var bracket models.Bracket
	if err := server.DB.First(&bracket, bracketID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
		return
	}

	existingLike := models.BracketLike{}
	err = server.DB.Where("user_id = ? AND bracket_id = ?", uid, uint(bid)).First(&existingLike).Error
	if err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "You have already liked this bracket"})
		return
	} else if err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error checking existing like"})
		return
	}

	like := models.BracketLike{
		UserID:    uid,
		BracketID: uint(bid),
	}
	if err := server.DB.Create(&like).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error liking bracket"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"status": http.StatusCreated, "response": "Bracket liked successfully"})
}

// UnLikeBracket allows a user to remove their like from a specific bracket
func (server *Server) UnLikeBracket(c *gin.Context) {
	tokenID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	uid := tokenID.(uint)

	bracketID := c.Param("id")
	bid, err := strconv.ParseUint(bracketID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bracket ID"})
		return
	}

	existingLike := models.BracketLike{}
	err = server.DB.Where("user_id = ? AND bracket_id = ?", uid, uint(bid)).First(&existingLike).Error
	if err == gorm.ErrRecordNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "Like not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error finding like"})
		return
	}

	if err := server.DB.Delete(&existingLike).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error unliking bracket"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "response": "Bracket unliked successfully"})
}

// GetBracketLikes retrieves likes for a bracket
func (server *Server) GetBracketLikes(c *gin.Context) {
	bracketID := c.Param("id")
	bid, err := strconv.ParseUint(bracketID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bracket ID"})
		return
	}

	likes := []models.BracketLike{}
	if err := server.DB.Where("bracket_id = ?", uint(bid)).Find(&likes).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No likes found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "response": likes})
}

// GetUserBracketLikes retrieves all bracket likes for a user
func (server *Server) GetUserBracketLikes(c *gin.Context) {
	userID := c.Param("id")
	uid, err := strconv.ParseUint(userID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	likes := []models.BracketLike{}
	err = server.DB.Where("user_id = ?", uint(uid)).Find(&likes).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving user bracket likes"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "response": likes})
}
