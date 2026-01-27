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

// popularMatchupRow represents a row from the popular_matchups_snapshot table
type popularMatchupRow struct {
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

// GetPopularMatchups godoc
// @Summary      Get popular matchups
// @Description  Get top engaged matchups
// @Tags         popular
// @Produce      json
// @Success      200  {object}  PopularMatchupsEnvelope
// @Failure      500  {object}  ErrorResponse
// @Router       /matchups/popular [get]
func (server *Server) GetPopularMatchups(c *gin.Context) {
	const limit = 5
	viewerID, hasViewer := optionalViewerID(c)
	isAdmin := httpctx.IsAdminRequest(c)
	cacheKey := fmt.Sprintf("popular_matchups:top5:viewer:%d:admin:%t", viewerID, isAdmin)
	ctx := context.Background()

	// 1. Try Redis first
	if cached, err := cache.Get(ctx, cacheKey); err == nil && cached != "" {
		c.Data(http.StatusOK, "application/json", []byte(cached))
		return
	}

	// 2. Fallback to DB
	var rows []popularMatchupRow
	err := server.DB.
		Table("popular_matchups_snapshot").
		Order("rank ASC").
		Limit(limit).
		Scan(&rows).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "failed to load popular matchups",
		})
		return
	}

	matchupIDs := make([]uint, 0, len(rows))
	authorIDs := make([]uint, 0, len(rows))
	bracketIDs := make([]uint, 0, len(rows))
	bracketAuthorIDs := make([]uint, 0, len(rows))
	for _, row := range rows {
		matchupIDs = append(matchupIDs, row.ID)
		authorIDs = append(authorIDs, row.AuthorID)
		if row.BracketID != nil {
			bracketIDs = append(bracketIDs, *row.BracketID)
		}
		if row.BracketAuthorID != nil {
			bracketAuthorIDs = append(bracketAuthorIDs, *row.BracketAuthorID)
		}
	}

	matchupPublicIDs := loadMatchupPublicIDMap(server.DB, matchupIDs)
	authorPublicIDs := loadUserPublicIDMap(server.DB, authorIDs)
	bracketPublicIDs := loadBracketPublicIDMap(server.DB, bracketIDs)
	bracketAuthorPublicIDs := loadUserPublicIDMap(server.DB, bracketAuthorIDs)

	filtered := make([]PopularMatchupDTO, 0, len(rows))
	for i := range rows {
		var matchup models.Matchup
		if err := server.DB.Preload("Author").First(&matchup, rows[i].ID).Error; err != nil {
			continue
		}
		allowed, _, err := canViewUserContent(server.DB, viewerID, hasViewer, &matchup.Author, matchup.Visibility, isAdmin)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  "error",
				"message": "failed to check matchup visibility",
			})
			return
		}
		if !allowed {
			continue
		}
		var bracketID *string
		if rows[i].BracketID != nil {
			if id := bracketPublicIDs[*rows[i].BracketID]; id != "" {
				bracketID = &id
			}
		}
		var bracketAuthorID *string
		if rows[i].BracketAuthorID != nil {
			if id := bracketAuthorPublicIDs[*rows[i].BracketAuthorID]; id != "" {
				bracketAuthorID = &id
			}
		}
		filtered = append(filtered, PopularMatchupDTO{
			ID:              matchupPublicIDs[rows[i].ID],
			Title:           rows[i].Title,
			AuthorID:        authorPublicIDs[rows[i].AuthorID],
			BracketID:       bracketID,
			BracketAuthorID: bracketAuthorID,
			Round:           rows[i].Round,
			CurrentRound:    rows[i].CurrentRound,
			Votes:           rows[i].Votes,
			Likes:           rows[i].Likes,
			Comments:        rows[i].Comments,
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
