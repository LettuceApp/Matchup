package controllers

import (
	"Matchup/api/auth"
	"Matchup/api/models"
	"Matchup/api/utils/formaterror"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (server *Server) LikeMatchup(c *gin.Context) {
	like := models.Like{}
	uid, err := auth.ExtractTokenID(c.Request)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

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

	// Create a new like
	like.UserID = uid
	like.MatchupID = uint(mid)
	_, err = like.SaveLike(server.DB)
	if err != nil {
		formattedError := formaterror.FormatError(err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": formattedError})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":   http.StatusCreated,
		"response": "Like added",
	})
}

func (server *Server) UnLikeMatchup(c *gin.Context) {
	uid, err := auth.ExtractTokenID(c.Request)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	matchupID := c.Param("id")
	mid, err := strconv.ParseUint(matchupID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid matchup ID"})
		return
	}

	like := models.Like{}
	err = server.DB.Model(&models.Like{}).Where("user_id = ? AND matchup_id = ?", uid, uint(mid)).First(&like).Error
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Like not found"})
		return
	}

	_, err = like.DeleteLike(server.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error deleting like"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "response": "Like removed"})
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
