package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// PopularMatchup represents a row from the popular_matchups table
type PopularMatchup struct {
	ID              uint    `json:"id" gorm:"column:matchup_id"`
	Title           string  `json:"title" gorm:"column:title"`
	AuthorID        uint    `json:"author_id" gorm:"column:author_id"`
	TotalVotes      int64   `json:"total_votes" gorm:"column:total_votes"`
	Likes           int64   `json:"likes" gorm:"column:likes"`
	Comments        int64   `json:"comments" gorm:"column:comments"`
	EngagementScore float64 `json:"engagement_score" gorm:"column:engagement_score"`
	Rank            int64   `json:"rank" gorm:"column:rank"`
}

// GetPopularMatchups returns the top popular matchups from the analytics table
func (server *Server) GetPopularMatchups(c *gin.Context) {
	const defaultLimit = 5

	// You can optionally read ?limit= param if you want, but we'll keep it fixed at 5
	var results []PopularMatchup

	// Map matchup_id -> id so the frontend sees the same "id" it expects
	tx := server.DB.
		Table("popular_matchups").
		Select(`
            matchup_id,
            title,
            author_id,
            total_votes,
            likes,
            comments,
            engagement_score,
            rank
        `).
		Order("rank ASC").
		Limit(defaultLimit)

	if err := tx.Scan(&results).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "failed to load popular matchups",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "success",
		"response": results,
	})
}
