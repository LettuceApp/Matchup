package controllers

import (
	"Matchup/cache"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// PopularMatchup represents a row from the popular_matchups_snapshot table
type PopularMatchup struct {
	ID              uint    `json:"id" gorm:"column:matchup_id"`
	Title           string  `json:"title" gorm:"column:title"`
	AuthorID        uint    `json:"author_id" gorm:"column:author_id"`
	BracketID       *uint   `json:"bracket_id" gorm:"column:bracket_id"`
	BracketAuthorID *uint   `json:"bracket_author_id" gorm:"column:bracket_author_id"`
	Round           *int    `json:"round" gorm:"column:round"`
	CurrentRound    *int    `json:"current_round" gorm:"column:current_round"`
	Votes           int64   `json:"votes" gorm:"column:votes"`
	Likes           int64   `json:"likes" gorm:"column:likes"`
	Comments        int64   `json:"comments" gorm:"column:comments"`
	EngagementScore float64 `json:"engagement_score" gorm:"column:engagement_score"`
	Rank            int64   `json:"rank" gorm:"column:rank"`
}

// GetPopularMatchups returns the top popular matchups from the analytics table
func (server *Server) GetPopularMatchups(c *gin.Context) {
	const limit = 5
	cacheKey := "popular_matchups:top3"
	ctx := context.Background()

	// 1. Try Redis first
	if cached, err := cache.Get(ctx, cacheKey); err == nil && cached != "" {
		c.Data(http.StatusOK, "application/json", []byte(cached))
		return
	}

	// 2. Fallback to DB
	var results []PopularMatchup
	err := server.DB.
		Table("popular_matchups_snapshot").
		Order("rank ASC").
		Limit(limit).
		Scan(&results).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "failed to load popular matchups",
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
