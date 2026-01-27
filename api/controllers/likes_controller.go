package controllers

import (
	"errors"
	"net/http"
	"time"

	"Matchup/models"
	httpctx "Matchup/utils/httpctx"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// LikeMatchup godoc
// @Summary      Like a matchup
// @Description  Like a matchup as the authenticated user
// @Tags         likes
// @Produce      json
// @Param        id   path      string  true  "Matchup ID"
// @Success      201  {object}  SimpleMessageResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Router       /matchups/{id}/likes [post]
// @Security     BearerAuth
func (server *Server) LikeMatchup(c *gin.Context) {

	// Get the authenticated user's ID
	tokenID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	uid := tokenID.(uint)

	matchupID := c.Param("id")
	matchupRecord, err := resolveMatchupByIdentifier(server.DB, matchupID)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid matchup ID"})
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Matchup not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Matchup not found"})
		}
		return
	}

	// Check if the matchup exists
	var matchup models.Matchup
	if err := server.DB.Preload("Author").First(&matchup, matchupRecord.ID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Matchup not found"})
		return
	}

	allowed, reason, err := canViewUserContent(server.DB, uid, true, &matchup.Author, matchup.Visibility, httpctx.IsAdminRequest(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to check matchup visibility"})
		return
	}
	if !allowed {
		respondVisibilityDenied(c, reason)
		return
	}

	// 🚫 Not activated
	if matchup.Status != matchupStatusCompleted && !isMatchupOpenStatus(matchup.Status) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Matchup is not active"})
		return
	}

	if matchup.Status != matchupStatusCompleted && matchup.EndTime != nil && time.Now().After(*matchup.EndTime) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Matchup has ended"})
		return
	}

	// 🧩 Bracket constraint
	if matchup.BracketID != nil && matchup.Status != matchupStatusCompleted {
		var bracket models.Bracket
		if err := server.DB.First(&bracket, *matchup.BracketID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
			return
		}

		if bracket.Status != "active" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Bracket is not active"})
			return
		}

		if matchup.Round == nil || *matchup.Round != bracket.CurrentRound {
			c.JSON(http.StatusForbidden, gin.H{"error": "Matchup is not in the active round"})
			return
		}
	}

	// Check if the user has already liked the matchup
	existingLike := models.Like{}
	err = server.DB.Where("user_id = ? AND matchup_id = ?", uid, matchupRecord.ID).First(&existingLike).Error
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
		MatchupID: matchupRecord.ID,
	}
	if err := server.DB.Create(&like).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error liking matchup"})
		return
	}
	if matchup.BracketID != nil {
		invalidateBracketSummaryCache(*matchup.BracketID)
	}
	invalidateHomeSummaryCache(matchup.AuthorID)

	c.JSON(http.StatusCreated, gin.H{"status": http.StatusCreated, "response": "Matchup liked successfully"})
}

// UnLikeMatchup godoc
// @Summary      Unlike a matchup
// @Description  Remove the authenticated user's like from a matchup
// @Tags         likes
// @Produce      json
// @Param        id   path      string  true  "Matchup ID"
// @Success      200  {object}  SimpleMessageResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /matchups/{id}/likes [delete]
// @Security     BearerAuth
func (server *Server) UnLikeMatchup(c *gin.Context) {
	// Get the authenticated user's ID
	tokenID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	uid := tokenID.(uint)

	matchupID := c.Param("id")
	matchupRecord, err := resolveMatchupByIdentifier(server.DB, matchupID)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid matchup ID"})
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Matchup not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Matchup not found"})
		}
		return
	}

	// Find the like by the user on the specific matchup
	existingLike := models.Like{}
	err = server.DB.Where("user_id = ? AND matchup_id = ?", uid, matchupRecord.ID).First(&existingLike).Error
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
	var matchup models.Matchup
	if err := server.DB.First(&matchup, matchupRecord.ID).Error; err == nil {
		if matchup.BracketID != nil {
			invalidateBracketSummaryCache(*matchup.BracketID)
		}
		invalidateHomeSummaryCache(matchup.AuthorID)
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "response": "Matchup unliked successfully"})
}

// GetLikes godoc
// @Summary      List matchup likes
// @Description  Get likes for a matchup
// @Tags         likes
// @Produce      json
// @Param        id   path      string  true  "Matchup ID"
// @Success      200  {object}  LikesListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /matchups/{id}/likes [get]
func (server *Server) GetLikes(c *gin.Context) {
	matchupID := c.Param("id")
	matchupRecord, err := resolveMatchupByIdentifier(server.DB, matchupID)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid matchup ID"})
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Matchup not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Matchup not found"})
		}
		return
	}

	var matchup models.Matchup
	if err := server.DB.Preload("Author").First(&matchup, matchupRecord.ID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Matchup not found"})
		return
	}

	viewerID, hasViewer := optionalViewerID(c)
	allowed, reason, err := canViewUserContent(server.DB, viewerID, hasViewer, &matchup.Author, matchup.Visibility, httpctx.IsAdminRequest(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to check matchup visibility"})
		return
	}
	if !allowed {
		respondVisibilityDenied(c, reason)
		return
	}

	like := models.Like{}
	likes, err := like.GetLikesInfo(server.DB, matchupRecord.ID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No likes found"})
		return
	}

	userIDs := make([]uint, 0, len(*likes))
	matchupIDs := make([]uint, 0, len(*likes))
	for _, likeEntry := range *likes {
		userIDs = append(userIDs, likeEntry.UserID)
		matchupIDs = append(matchupIDs, likeEntry.MatchupID)
	}

	userPublicIDs := loadUserPublicIDMap(server.DB, userIDs)
	matchupPublicIDs := loadMatchupPublicIDMap(server.DB, matchupIDs)

	response := make([]LikeDTO, len(*likes))
	for i, likeEntry := range *likes {
		response[i] = likeToDTO(likeEntry, userPublicIDs[likeEntry.UserID], matchupPublicIDs[likeEntry.MatchupID])
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "response": response})
}

// GetUserLikes godoc
// @Summary      List user likes
// @Description  Get likes made by a user
// @Tags         likes
// @Produce      json
// @Param        id   path      string  true  "User ID"
// @Success      200  {object}  LikesListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /users/{id}/likes [get]
// GetUserLikes retrieves all likes made by a specific user
func (server *Server) GetUserLikes(c *gin.Context) {
	user, err := resolveUserByIdentifier(server.DB, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	likes := []models.Like{}
	err = server.DB.Where("user_id = ?", user.ID).Find(&likes).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving user likes"})
		return
	}

	matchupIDs := make([]uint, 0, len(likes))
	for _, likeEntry := range likes {
		matchupIDs = append(matchupIDs, likeEntry.MatchupID)
	}

	userPublicIDs := loadUserPublicIDMap(server.DB, []uint{user.ID})
	matchupPublicIDs := loadMatchupPublicIDMap(server.DB, matchupIDs)

	response := make([]LikeDTO, len(likes))
	for i, likeEntry := range likes {
		response[i] = likeToDTO(likeEntry, userPublicIDs[likeEntry.UserID], matchupPublicIDs[likeEntry.MatchupID])
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "response": response})
}
