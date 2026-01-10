package controllers

import (
	"Matchup/cache"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"Matchup/models"

	"github.com/gin-gonic/gin"
)

type BracketSummary struct {
	Bracket       *models.Bracket  `json:"bracket"`
	Matchups      []models.Matchup `json:"matchups"`
	LikedMatchups []uint           `json:"liked_matchup_ids"`
	LikedBracket  bool             `json:"liked_bracket"`
}

func (s *Server) GetBracketSummary(c *gin.Context) {
	bracketID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bracket ID"})
		return
	}

	viewerID := uint(0)
	if viewerParam := c.Query("viewer_id"); viewerParam != "" {
		if vid, err := strconv.ParseUint(viewerParam, 10, 32); err == nil {
			viewerID = uint(vid)
		}
	}

	cacheKey := fmt.Sprintf("bracket_summary:%d:%d", bracketID, viewerID)
	ctx := context.Background()
	if cached, err := cache.Get(ctx, cacheKey); err == nil && cached != "" {
		c.Data(http.StatusOK, "application/json", []byte(cached))
		return
	}

	var bracket models.Bracket
	found, err := bracket.FindBracketByID(s.DB, uint(bracketID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
		return
	}
	bracket = *found

	if bracket.Status == "active" &&
		bracket.AdvanceMode == "timer" &&
		bracket.RoundEndsAt != nil &&
		!time.Now().Before(*bracket.RoundEndsAt) {
		if _, err := s.advanceBracketInternal(s.DB, &bracket); err != nil {
			log.Printf("auto advance bracket %d: %v", bracket.ID, err)
		}
	}

	matchups, err := models.FindMatchupsByBracket(s.DB, uint(bracketID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load matchups"})
		return
	}

	for i := range matchups {
		if err := dedupeBracketMatchupItems(s.DB, &matchups[i]); err != nil {
			log.Printf("dedupe bracket matchup %d: %v", matchups[i].ID, err)
		}
	}

	likedMatchups := []uint{}
	likedBracket := false
	if viewerID > 0 {
		if err := s.DB.Model(&models.Like{}).
			Where("user_id = ? AND matchup_id IN (SELECT id FROM matchups WHERE bracket_id = ?)", viewerID, bracketID).
			Pluck("matchup_id", &likedMatchups).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load user likes"})
			return
		}

		var count int64
		if err := s.DB.Model(&models.BracketLike{}).
			Where("user_id = ? AND bracket_id = ?", viewerID, bracketID).
			Count(&count).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load bracket likes"})
			return
		}
		likedBracket = count > 0
	}

	response := BracketSummary{
		Bracket:       &bracket,
		Matchups:      matchups,
		LikedMatchups: likedMatchups,
		LikedBracket:  likedBracket,
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
