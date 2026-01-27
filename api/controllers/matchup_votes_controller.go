package controllers

import (
	"Matchup/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetUserMatchupVotes godoc
// @Summary      List user matchup votes
// @Description  Get votes made by a user
// @Tags         votes
// @Produce      json
// @Param        id   path      string  true  "User ID"
// @Success      200  {object}  MatchupVotesListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /users/{id}/matchup_votes [get]
func (server *Server) GetUserMatchupVotes(c *gin.Context) {
	user, err := resolveUserByIdentifier(server.DB, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	votes := []models.MatchupVote{}
	if err := server.DB.Where("user_id = ?", user.ID).Find(&votes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving user matchup votes"})
		return
	}

	response := make([]MatchupVoteDTO, len(votes))
	for i, vote := range votes {
		userPublicID := user.PublicID
		response[i] = matchupVoteToDTO(vote, &userPublicID)
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "response": response})
}
