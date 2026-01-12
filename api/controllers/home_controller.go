package controllers

import (
	"Matchup/cache"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type HomeSummary struct {
	PopularMatchups  []PopularMatchup `json:"popular_matchups"`
	PopularBrackets  []PopularBracket `json:"popular_brackets"`
	TotalEngagements float64          `json:"total_engagements"`
}

func (server *Server) GetHomeSummary(c *gin.Context) {
	userIDParam := c.Query("user_id")
	var userID uint
	if userIDParam != "" {
		if uid, err := strconv.ParseUint(userIDParam, 10, 32); err == nil {
			userID = uint(uid)
		}
	}

	cacheKey := fmt.Sprintf("home_summary:%d", userID)
	ctx := context.Background()
	if cached, err := cache.Get(ctx, cacheKey); err == nil && cached != "" {
		c.Data(http.StatusOK, "application/json", []byte(cached))
		return
	}

	popularMatchups := []PopularMatchup{}
	if err := server.DB.
		Table("popular_matchups_snapshot").
		Order("rank ASC").
		Limit(5).
		Scan(&popularMatchups).Error; err != nil {
		if !isMissingRelation(err, "popular_matchups_snapshot") {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  "error",
				"message": "failed to load popular matchups",
			})
			return
		}
		log.Printf("popular matchups snapshot missing: %v", err)
	}

	popularBrackets := []PopularBracket{}
	if err := server.DB.
		Table("popular_brackets_snapshot").
		Order("rank ASC").
		Limit(5).
		Scan(&popularBrackets).Error; err != nil {
		if !isMissingRelation(err, "popular_brackets_snapshot") {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  "error",
				"message": "failed to load popular brackets",
			})
			return
		}
		log.Printf("popular brackets snapshot missing: %v", err)
	}

	totalEngagements := 0.0
	if userID > 0 {
		total, err := server.calculateUserEngagement(userID)
		if err != nil {
			log.Printf("calculate user engagement: %v", err)
		} else {
			totalEngagements = total
		}
	}

	response := HomeSummary{
		PopularMatchups:  popularMatchups,
		PopularBrackets:  popularBrackets,
		TotalEngagements: totalEngagements,
	}

	if jsonBytes, err := json.Marshal(gin.H{
		"status":   "success",
		"response": response,
	}); err == nil {
		_ = cache.Set(ctx, cacheKey, jsonBytes, 60*time.Second)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "success",
		"response": response,
	})
}

func isMissingRelation(err error, relation string) bool {
	if err == nil {
		return false
	}
	errLower := strings.ToLower(err.Error())
	return strings.Contains(errLower, "does not exist") && strings.Contains(errLower, strings.ToLower(relation))
}

func (server *Server) calculateUserEngagement(userID uint) (float64, error) {
	var totalVotes int64
	if err := server.DB.Raw(`
		SELECT COALESCE(SUM(mi.votes), 0)
		FROM matchup_items mi
		JOIN matchups m ON m.id = mi.matchup_id
		WHERE m.author_id = ?
	`, userID).Scan(&totalVotes).Error; err != nil {
		return 0, err
	}

	var selfVotes int64
	if err := server.DB.Raw(`
		SELECT COALESCE(COUNT(*), 0)
		FROM matchup_votes mv
		JOIN matchups m ON m.id = mv.matchup_id
		WHERE m.author_id = ? AND mv.user_id = ?
	`, userID, userID).Scan(&selfVotes).Error; err != nil {
		return 0, err
	}

	var likes int64
	if err := server.DB.Raw(`
		SELECT COALESCE(COUNT(*), 0)
		FROM likes l
		JOIN matchups m ON m.id = l.matchup_id
		WHERE m.author_id = ? AND l.user_id <> ?
	`, userID, userID).Scan(&likes).Error; err != nil {
		return 0, err
	}

	var comments int64
	if err := server.DB.Raw(`
		SELECT COALESCE(COUNT(*), 0)
		FROM comments c
		JOIN matchups m ON m.id = c.matchup_id
		WHERE m.author_id = ? AND c.user_id <> ?
	`, userID, userID).Scan(&comments).Error; err != nil {
		return 0, err
	}

	var bracketLikes int64
	if err := server.DB.Raw(`
		SELECT COALESCE(COUNT(*), 0)
		FROM bracket_likes bl
		JOIN brackets b ON b.id = bl.bracket_id
		WHERE b.author_id = ? AND bl.user_id <> ?
	`, userID, userID).Scan(&bracketLikes).Error; err != nil {
		return 0, err
	}

	var bracketComments int64
	if err := server.DB.Raw(`
		SELECT COALESCE(COUNT(*), 0)
		FROM bracket_comments bc
		JOIN brackets b ON b.id = bc.bracket_id
		WHERE b.author_id = ? AND bc.user_id <> ?
	`, userID, userID).Scan(&bracketComments).Error; err != nil {
		return 0, err
	}

	votes := totalVotes - selfVotes
	if votes < 0 {
		votes = 0
	}

	return float64(votes)*2 +
		float64(likes)*3 +
		float64(comments)*.5 +
		float64(bracketLikes)*3 +
		float64(bracketComments)*.5, nil
}
