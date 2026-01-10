package controllers

import (
	"Matchup/models"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GetUserMatchupVotes retrieves all matchup votes made by a specific user.
func (server *Server) GetUserMatchupVotes(c *gin.Context) {
	userID := c.Param("id")
	uid, err := strconv.ParseUint(userID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	votes := []models.MatchupVote{}
	if err := server.DB.Where("user_id = ?", uint(uid)).Find(&votes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving user matchup votes"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "response": votes})
}
