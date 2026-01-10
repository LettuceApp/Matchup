package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"Matchup/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func toAdminBracketSummary(bracket *models.Bracket, likesCount int64) map[string]interface{} {
	authorUsername := ""
	if bracket.Author.ID != 0 {
		authorUsername = bracket.Author.Username
	}

	return map[string]interface{}{
		"id":              bracket.ID,
		"title":           bracket.Title,
		"author_id":       bracket.AuthorID,
		"author_username": authorUsername,
		"status":          bracket.Status,
		"current_round":   bracket.CurrentRound,
		"likes_count":     likesCount,
		"created_at":      bracket.CreatedAt,
		"updated_at":      bracket.UpdatedAt,
	}
}

// AdminListBrackets returns a paginated list of brackets for moderation tools.
func (server *Server) AdminListBrackets(c *gin.Context) {
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

	query := server.DB.Model(&models.Bracket{})
	if search != "" {
		like := fmt.Sprintf("%%%s%%", search)
		query = query.Where("title ILIKE ?", like)
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to count brackets"})
		return
	}

	var brackets []models.Bracket
	if err := query.
		Preload("Author").
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&brackets).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to fetch brackets"})
		return
	}

	bracketResponses := make([]map[string]interface{}, len(brackets))
	for i := range brackets {
		var likesCount int64
		server.DB.Model(&models.BracketLike{}).Where("bracket_id = ?", brackets[i].ID).Count(&likesCount)
		bracketResponses[i] = toAdminBracketSummary(&brackets[i], likesCount)
	}

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"response": gin.H{
			"brackets":   bracketResponses,
			"pagination": buildPagination(page, limit, total),
		},
	})
}

// AdminDeleteBracket deletes a bracket and its matchups.
func (server *Server) AdminDeleteBracket(c *gin.Context) {
	bracketID := c.Param("id")
	bid64, err := strconv.ParseUint(bracketID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bracket ID"})
		return
	}
	bid := uint(bid64)

	var bracket models.Bracket
	if err := server.DB.First(&bracket, bid).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving bracket"})
		return
	}

	if err := server.deleteBracketCascade(&bracket); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error deleting bracket"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Bracket deleted"})
}
