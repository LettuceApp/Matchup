package controllers

import (
	"Matchup/cache"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"Matchup/models"
	httpctx "Matchup/utils/httpctx"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetBracketSummary godoc
// @Summary      Get bracket summary
// @Description  Get bracket, matchups, and viewer-specific likes
// @Tags         brackets
// @Produce      json
// @Param        id         path      string  true   "Bracket ID"
// @Param        viewer_id  query     string  false  "Viewer ID"
// @Success      200        {object}  BracketSummaryEnvelope
// @Failure      400        {object}  ErrorResponse
// @Failure      404        {object}  ErrorResponse
// @Failure      500        {object}  ErrorResponse
// @Router       /brackets/{id}/summary [get]
func (s *Server) GetBracketSummary(c *gin.Context) {
	bracketRecord, err := resolveBracketByIdentifier(s.DB, c.Param("id"))
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

	accessViewerID, hasViewer := optionalViewerID(c)
	likesViewerID := accessViewerID
	if likesViewerID == 0 {
		if viewerParam := c.Query("viewer_id"); viewerParam != "" {
			if viewer, err := resolveUserByIdentifier(s.DB, viewerParam); err == nil {
				likesViewerID = viewer.ID
			}
		}
	}

	var bracket models.Bracket
	found, err := bracket.FindBracketByID(s.DB, bracketRecord.ID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
		return
	}
	bracket = *found

	allowed, reason, err := canViewUserContent(s.DB, accessViewerID, hasViewer, &bracket.Author, bracket.Visibility, httpctx.IsAdminRequest(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to check bracket visibility"})
		return
	}
	if !allowed {
		respondVisibilityDenied(c, reason)
		return
	}

	cacheKey := fmt.Sprintf("bracket_summary:%d:%d", bracketRecord.ID, likesViewerID)
	ctx := context.Background()
	shouldCache := !(bracket.Status == "active" && bracket.AdvanceMode == "timer")
	if shouldCache {
		if cached, err := cache.Get(ctx, cacheKey); err == nil && cached != "" {
			c.Data(http.StatusOK, "application/json", []byte(cached))
			return
		}
	}

	if bracket.Status == "active" &&
		bracket.AdvanceMode == "timer" &&
		bracket.RoundEndsAt != nil &&
		!time.Now().Before(*bracket.RoundEndsAt) {
		if _, err := s.advanceBracketInternal(s.DB, &bracket); err != nil {
			log.Printf("auto advance bracket %d: %v", bracket.ID, err)
		}
	}

	matchups, err := models.FindMatchupsByBracket(s.DB, bracketRecord.ID)
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
	var bracketLikesCount int64
	if likesViewerID > 0 {
		if err := s.DB.Model(&models.Like{}).
			Where("user_id = ? AND matchup_id IN (SELECT id FROM matchups WHERE bracket_id = ?)", likesViewerID, bracketRecord.ID).
			Pluck("matchup_id", &likedMatchups).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load user likes"})
			return
		}

		var count int64
		if err := s.DB.Model(&models.BracketLike{}).
			Where("user_id = ? AND bracket_id = ?", likesViewerID, bracketRecord.ID).
			Count(&count).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load bracket likes"})
			return
		}
		likedBracket = count > 0
	}
	s.DB.Model(&models.BracketLike{}).
		Where("bracket_id = ?", bracketRecord.ID).
		Count(&bracketLikesCount)
	bracket.LikesCount = bracketLikesCount

	matchupResponses := make([]MatchupDTO, 0, len(matchups))
	matchupPublicIDs := make(map[uint]string, len(matchups))
	for i := range matchups {
		var likesCount int64
		s.DB.Model(&models.Like{}).Where("matchup_id = ?", matchups[i].ID).Count(&likesCount)
		matchupResponses = append(matchupResponses, matchupToDTO(s.DB, &matchups[i], []models.Comment{}, likesCount))
		matchupPublicIDs[matchups[i].ID] = matchups[i].PublicID
	}

	likedMatchupIDs := make([]string, 0, len(likedMatchups))
	for _, id := range likedMatchups {
		if publicID, ok := matchupPublicIDs[id]; ok {
			likedMatchupIDs = append(likedMatchupIDs, publicID)
		}
	}

	response := BracketSummaryDTO{
		Bracket:       bracketToDTO(s.DB, &bracket),
		Matchups:      matchupResponses,
		LikedMatchups: likedMatchupIDs,
		LikedBracket:  likedBracket,
	}

	if shouldCache {
		if jsonBytes, err := json.Marshal(gin.H{
			"status":   "success",
			"response": response,
		}); err == nil {
			_ = cache.Set(ctx, cacheKey, jsonBytes, 60*time.Second)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "success",
		"response": response,
	})
}
