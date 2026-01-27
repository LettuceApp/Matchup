package controllers

import (
	"errors"
	"net/http"

	"Matchup/models"
	httpctx "Matchup/utils/httpctx"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// LikeBracket godoc
// @Summary      Like a bracket
// @Description  Like a bracket as the authenticated user
// @Tags         bracket-likes
// @Produce      json
// @Param        id   path      string  true  "Bracket ID"
// @Success      201  {object}  SimpleMessageResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Router       /brackets/{id}/likes [post]
// @Security     BearerAuth
func (server *Server) LikeBracket(c *gin.Context) {
	tokenID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	uid := tokenID.(uint)

	bracketID := c.Param("id")
	bracketRecord, err := resolveBracketByIdentifier(server.DB, bracketID)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bracket ID"})
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Bracket not found"})
		}
		return
	}

	var bracket models.Bracket
	if err := server.DB.Preload("Author").First(&bracket, bracketRecord.ID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
		return
	}

	allowed, reason, err := canViewUserContent(server.DB, uid, true, &bracket.Author, bracket.Visibility, httpctx.IsAdminRequest(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to check bracket visibility"})
		return
	}
	if !allowed {
		respondVisibilityDenied(c, reason)
		return
	}

	existingLike := models.BracketLike{}
	err = server.DB.Where("user_id = ? AND bracket_id = ?", uid, bracketRecord.ID).First(&existingLike).Error
	if err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "You have already liked this bracket"})
		return
	} else if err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error checking existing like"})
		return
	}

	like := models.BracketLike{
		UserID:    uid,
		BracketID: bracketRecord.ID,
	}
	if err := server.DB.Create(&like).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error liking bracket"})
		return
	}
	invalidateBracketSummaryCache(bracket.ID)
	invalidateHomeSummaryCache(bracket.AuthorID)

	c.JSON(http.StatusCreated, gin.H{"status": http.StatusCreated, "response": "Bracket liked successfully"})
}

// UnLikeBracket godoc
// @Summary      Unlike a bracket
// @Description  Remove the authenticated user's like from a bracket
// @Tags         bracket-likes
// @Produce      json
// @Param        id   path      string  true  "Bracket ID"
// @Success      200  {object}  SimpleMessageResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /brackets/{id}/likes [delete]
// @Security     BearerAuth
func (server *Server) UnLikeBracket(c *gin.Context) {
	tokenID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	uid := tokenID.(uint)

	bracketID := c.Param("id")
	bracketRecord, err := resolveBracketByIdentifier(server.DB, bracketID)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bracket ID"})
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Bracket not found"})
		}
		return
	}

	existingLike := models.BracketLike{}
	err = server.DB.Where("user_id = ? AND bracket_id = ?", uid, bracketRecord.ID).First(&existingLike).Error
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
	var bracket models.Bracket
	if err := server.DB.First(&bracket, bracketRecord.ID).Error; err == nil {
		invalidateBracketSummaryCache(bracket.ID)
		invalidateHomeSummaryCache(bracket.AuthorID)
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "response": "Bracket unliked successfully"})
}

// GetBracketLikes godoc
// @Summary      List bracket likes
// @Description  Get likes for a bracket
// @Tags         bracket-likes
// @Produce      json
// @Param        id   path      string  true  "Bracket ID"
// @Success      200  {object}  BracketLikesListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /brackets/{id}/likes [get]
func (server *Server) GetBracketLikes(c *gin.Context) {
	bracketID := c.Param("id")
	bracketRecord, err := resolveBracketByIdentifier(server.DB, bracketID)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bracket ID"})
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Bracket not found"})
		}
		return
	}

	var bracket models.Bracket
	if err := server.DB.Preload("Author").First(&bracket, bracketRecord.ID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
		return
	}

	viewerID, hasViewer := optionalViewerID(c)
	allowed, reason, err := canViewUserContent(server.DB, viewerID, hasViewer, &bracket.Author, bracket.Visibility, httpctx.IsAdminRequest(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to check bracket visibility"})
		return
	}
	if !allowed {
		respondVisibilityDenied(c, reason)
		return
	}

	likes := []models.BracketLike{}
	if err := server.DB.Where("bracket_id = ?", bracketRecord.ID).Find(&likes).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No likes found"})
		return
	}

	userIDs := make([]uint, 0, len(likes))
	bracketIDs := make([]uint, 0, len(likes))
	for _, likeEntry := range likes {
		userIDs = append(userIDs, likeEntry.UserID)
		bracketIDs = append(bracketIDs, likeEntry.BracketID)
	}

	userPublicIDs := loadUserPublicIDMap(server.DB, userIDs)
	bracketPublicIDs := loadBracketPublicIDMap(server.DB, bracketIDs)

	response := make([]BracketLikeDTO, len(likes))
	for i, likeEntry := range likes {
		response[i] = bracketLikeToDTO(likeEntry, userPublicIDs[likeEntry.UserID], bracketPublicIDs[likeEntry.BracketID])
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "response": response})
}

// GetUserBracketLikes godoc
// @Summary      List user bracket likes
// @Description  Get bracket likes for a user
// @Tags         bracket-likes
// @Produce      json
// @Param        id   path      string  true  "User ID"
// @Success      200  {object}  BracketLikesListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /users/{id}/bracket_likes [get]
func (server *Server) GetUserBracketLikes(c *gin.Context) {
	user, err := resolveUserByIdentifier(server.DB, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	likes := []models.BracketLike{}
	err = server.DB.Where("user_id = ?", user.ID).Find(&likes).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving user bracket likes"})
		return
	}

	bracketIDs := make([]uint, 0, len(likes))
	for _, likeEntry := range likes {
		bracketIDs = append(bracketIDs, likeEntry.BracketID)
	}

	userPublicIDs := loadUserPublicIDMap(server.DB, []uint{user.ID})
	bracketPublicIDs := loadBracketPublicIDMap(server.DB, bracketIDs)

	response := make([]BracketLikeDTO, len(likes))
	for i, likeEntry := range likes {
		response[i] = bracketLikeToDTO(likeEntry, userPublicIDs[likeEntry.UserID], bracketPublicIDs[likeEntry.BracketID])
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "response": response})
}
