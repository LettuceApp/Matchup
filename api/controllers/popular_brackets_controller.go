package controllers

import (
	"Matchup/cache"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// PopularBracket represents a row from the popular_brackets_snapshot table
type PopularBracket struct {
	ID              uint    `json:"id" gorm:"column:bracket_id"`
	Title           string  `json:"title" gorm:"column:title"`
	AuthorID        uint    `json:"author_id" gorm:"column:author_id"`
	CurrentRound    int     `json:"current_round" gorm:"column:current_round"`
	MatchupCount    int64   `json:"matchup_count" gorm:"column:matchup_count"`
	EngagementScore float64 `json:"engagement_score" gorm:"column:bracket_engagement_score"`
	Rank            int64   `json:"rank" gorm:"column:rank"`
}

// GetPopularBrackets returns the top popular brackets from the analytics table
func (server *Server) GetPopularBrackets(c *gin.Context) {
	const limit = 5
	cacheKey := "popular_brackets:top5"
	ctx := context.Background()

	// 1. Try Redis first
	if cached, err := cache.Get(ctx, cacheKey); err == nil && cached != "" {
		c.Data(http.StatusOK, "application/json", []byte(cached))
		return
	}

	// 2. Fallback to DB
	var results []PopularBracket
	err := server.DB.
		Table("popular_brackets_snapshot").
		Order("rank ASC").
		Limit(limit).
		Scan(&results).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "failed to load popular brackets",
		})
		return
	}

	// 3. Cache result (short TTL)
	if jsonBytes, err := json.Marshal(gin.H{
		"status":   "success",
		"response": results,
	}); err == nil {
		_ = cache.Set(ctx, cacheKey, jsonBytes, 60*time.Second)
	}

	// 4. Return response
	c.JSON(http.StatusOK, gin.H{
		"status":   "success",
		"response": results,
	})
}
