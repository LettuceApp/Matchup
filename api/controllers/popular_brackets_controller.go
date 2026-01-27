package controllers

import (
	"Matchup/cache"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"Matchup/models"
	httpctx "Matchup/utils/httpctx"

	"github.com/gin-gonic/gin"
)

// popularBracketRow represents a row from the popular_brackets_snapshot table
type popularBracketRow struct {
	ID              uint    `json:"id" gorm:"column:bracket_id"`
	Title           string  `json:"title" gorm:"column:title"`
	AuthorID        uint    `json:"author_id" gorm:"column:author_id"`
	CurrentRound    int     `json:"current_round" gorm:"column:current_round"`
	MatchupCount    int64   `json:"matchup_count" gorm:"column:matchup_count"`
	EngagementScore float64 `json:"engagement_score" gorm:"column:bracket_engagement_score"`
	Rank            int64   `json:"rank" gorm:"column:rank"`
}

// GetPopularBrackets godoc
// @Summary      Get popular brackets
// @Description  Get top engaged brackets
// @Tags         popular
// @Produce      json
// @Success      200  {object}  PopularBracketsEnvelope
// @Failure      500  {object}  ErrorResponse
// @Router       /brackets/popular [get]
func (server *Server) GetPopularBrackets(c *gin.Context) {
	const limit = 5
	viewerID, hasViewer := optionalViewerID(c)
	isAdmin := httpctx.IsAdminRequest(c)
	cacheKey := fmt.Sprintf("popular_brackets:top5:viewer:%d:admin:%t", viewerID, isAdmin)
	ctx := context.Background()

	// 1. Try Redis first
	if cached, err := cache.Get(ctx, cacheKey); err == nil && cached != "" {
		c.Data(http.StatusOK, "application/json", []byte(cached))
		return
	}

	// 2. Fallback to DB
	var rows []popularBracketRow
	err := server.DB.
		Table("popular_brackets_snapshot").
		Order("rank ASC").
		Limit(limit).
		Scan(&rows).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "failed to load popular brackets",
		})
		return
	}

	bracketIDs := make([]uint, 0, len(rows))
	authorIDs := make([]uint, 0, len(rows))
	for _, row := range rows {
		bracketIDs = append(bracketIDs, row.ID)
		authorIDs = append(authorIDs, row.AuthorID)
	}

	bracketPublicIDs := loadBracketPublicIDMap(server.DB, bracketIDs)
	authorPublicIDs := loadUserPublicIDMap(server.DB, authorIDs)

	filtered := make([]PopularBracketDTO, 0, len(rows))
	for i := range rows {
		var bracket models.Bracket
		if err := server.DB.Preload("Author").First(&bracket, rows[i].ID).Error; err != nil {
			continue
		}
		allowed, _, err := canViewUserContent(server.DB, viewerID, hasViewer, &bracket.Author, bracket.Visibility, isAdmin)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  "error",
				"message": "failed to check bracket visibility",
			})
			return
		}
		if !allowed {
			continue
		}
		if bracket.Status != "active" {
			continue
		}
		likesCount := int64(0)
		if err := server.DB.Model(&models.BracketLike{}).
			Where("bracket_id = ?", bracket.ID).
			Count(&likesCount).Error; err != nil {
			likesCount = 0
		}
		commentsCount := int64(0)
		if err := server.DB.Model(&models.BracketComment{}).
			Where("bracket_id = ?", bracket.ID).
			Count(&commentsCount).Error; err != nil {
			commentsCount = 0
		}
		filtered = append(filtered, PopularBracketDTO{
			ID:              bracketPublicIDs[rows[i].ID],
			Title:           rows[i].Title,
			AuthorID:        authorPublicIDs[rows[i].AuthorID],
			CurrentRound:    rows[i].CurrentRound,
			Size:            bracket.Size,
			Votes:           0,
			Likes:           likesCount,
			Comments:        commentsCount,
			EngagementScore: rows[i].EngagementScore,
			Rank:            rows[i].Rank,
		})
	}

	// 3. Cache result (short TTL)
	if jsonBytes, err := json.Marshal(gin.H{
		"status":   "success",
		"response": filtered,
	}); err == nil {
		_ = cache.Set(ctx, cacheKey, jsonBytes, 60*time.Second)
	}

	// 4. Return response
	c.JSON(http.StatusOK, gin.H{
		"status":   "success",
		"response": filtered,
	})
}
