package controllers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"Matchup/models"
	httpctx "Matchup/utils/httpctx"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func ptrUint(v uint) *uint { return &v }
func ptrInt(v int) *int    { return &v }

const defaultRoundDurationSeconds = 86400

func setBracketRoundWindow(bracket *models.Bracket, now time.Time) {
	if bracket.AdvanceMode != "timer" {
		bracket.RoundStartedAt = nil
		bracket.RoundEndsAt = nil
		return
	}

	if bracket.RoundDurationSeconds <= 0 {
		bracket.RoundDurationSeconds = defaultRoundDurationSeconds
	}

	bracket.RoundStartedAt = &now
	end := now.Add(time.Duration(bracket.RoundDurationSeconds) * time.Second)
	bracket.RoundEndsAt = &end
}

func syncBracketMatchups(tx *gorm.DB, bracket *models.Bracket) error {
	if err := tx.Model(&models.Matchup{}).
		Where("bracket_id = ? AND round < ?", bracket.ID, bracket.CurrentRound).
		Update("status", matchupStatusCompleted).Error; err != nil {
		return err
	}

	currentUpdates := map[string]interface{}{
		"status": matchupStatusActive,
	}
	if bracket.AdvanceMode == "timer" {
		currentUpdates["start_time"] = bracket.RoundStartedAt
		currentUpdates["end_time"] = bracket.RoundEndsAt
	} else {
		currentUpdates["start_time"] = nil
		currentUpdates["end_time"] = nil
	}

	if err := tx.Model(&models.Matchup{}).
		Where("bracket_id = ? AND round = ?", bracket.ID, bracket.CurrentRound).
		Updates(currentUpdates).Error; err != nil {
		return err
	}

	if err := tx.Model(&models.Matchup{}).
		Where("bracket_id = ? AND round > ?", bracket.ID, bracket.CurrentRound).
		Updates(map[string]interface{}{
			"status":     matchupStatusDraft,
			"start_time": nil,
			"end_time":   nil,
		}).Error; err != nil {
		return err
	}

	return nil
}

//
// ===============================
// ROUND 1 GENERATION
// ===============================
//

func groupMatchupsByRound(matchups []models.Matchup) map[int][]models.Matchup {
	rounds := make(map[int][]models.Matchup)
	for _, m := range matchups {
		if m.Round != nil {
			rounds[*m.Round] = append(rounds[*m.Round], m)
		}
	}
	return rounds
}

func formatSeedLabel(seed int, name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Sprintf("Seed %d", seed)
	}
	trimmed = stripSeedPrefix(trimmed)
	if trimmed == "" {
		return fmt.Sprintf("Seed %d", seed)
	}
	return fmt.Sprintf("Seed %d - %s", seed, trimmed)
}

func stripSeedPrefix(label string) string {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" {
		return ""
	}

	for {
		lower := strings.ToLower(trimmed)
		if !strings.HasPrefix(lower, "seed") {
			return trimmed
		}

		i := len("seed")
		for i < len(trimmed) && trimmed[i] == ' ' {
			i++
		}
		startDigits := i
		for i < len(trimmed) && trimmed[i] >= '0' && trimmed[i] <= '9' {
			i++
		}
		if startDigits == i {
			return trimmed
		}
		for i < len(trimmed) && trimmed[i] == ' ' {
			i++
		}
		if i < len(trimmed) && (trimmed[i] == '-' || trimmed[i] == ':') {
			i++
			for i < len(trimmed) && trimmed[i] == ' ' {
				i++
			}
		}
		trimmed = strings.TrimSpace(trimmed[i:])
		if trimmed == "" {
			return ""
		}
	}
}

func generateFullBracket(db *gorm.DB, bracket models.Bracket, entries []string) {
	totalRounds := 0
	for (1 << totalRounds) < bracket.Size {
		totalRounds++
	}

	for round := 1; round <= totalRounds; round++ {
		matchupsThisRound := bracket.Size / (1 << round)

		for i := 0; i < matchupsThisRound; i++ {
			matchup := models.Matchup{
				Title:     fmt.Sprintf("Round %d - Match %d", round, i+1),
				Content:   "Bracket matchup",
				AuthorID:  bracket.AuthorID,
				BracketID: &bracket.ID,
				Round:     ptrInt(round),
			}

			// Only round 1 gets items immediately
			if round == 1 {
				seedA := i + 1
				seedB := bracket.Size - i
				var nameA, nameB string
				if seedA-1 < len(entries) {
					nameA = entries[seedA-1]
				}
				if seedB-1 < len(entries) {
					nameB = entries[seedB-1]
				}

				matchup.Items = []models.MatchupItem{
					{Item: formatSeedLabel(seedA, nameA)},
					{Item: formatSeedLabel(seedB, nameB)},
				}
			}

			matchup.Prepare()
			db.Create(&matchup)
		}
	}
}

//
// ===============================
// CREATE BRACKET
// ===============================
//

// CreateBracket godoc
// @Summary      Create a bracket
// @Description  Create a new bracket with optional seeded entries
// @Tags         brackets
// @Accept       json
// @Produce      json
// @Param        id       path      string                  true  "User ID"
// @Param        bracket  body      BracketCreateRequest  true  "Bracket payload"
// @Success      201      {object}  BracketResponseEnvelope
// @Failure      400      {object}  ErrorResponse
// @Failure      401      {object}  ErrorResponse
// @Router       /users/{id}/brackets [post]
// @Security     BearerAuth
func (s *Server) CreateBracket(c *gin.Context) {
	userID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var input struct {
		Title                string   `json:"title" binding:"required"`
		Description          string   `json:"description"`
		Size                 int      `json:"size" binding:"required,oneof=4 8 16 32 64"`
		AdvanceMode          string   `json:"advance_mode"`
		DurationMinutes      *int     `json:"duration_minutes"`
		RoundDurationSeconds *int     `json:"round_duration_seconds"`
		Visibility           string   `json:"visibility"`
		Entries              []string `json:"entries"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	roundDurationSeconds := 0
	if input.RoundDurationSeconds != nil {
		roundDurationSeconds = *input.RoundDurationSeconds
	} else if input.DurationMinutes != nil {
		roundDurationSeconds = *input.DurationMinutes * 60
	}
	if roundDurationSeconds <= 0 {
		roundDurationSeconds = defaultRoundDurationSeconds
	}

	advanceMode := input.AdvanceMode
	if advanceMode == "" {
		advanceMode = "manual"
	}

	bracket := models.Bracket{
		Title:                input.Title,
		Description:          input.Description,
		AuthorID:             userID,
		Size:                 input.Size, // ✅ STORED
		CurrentRound:         1,
		Status:               "draft",
		AdvanceMode:          advanceMode,
		RoundDurationSeconds: roundDurationSeconds,
		Visibility:           normalizeVisibility(input.Visibility),
	}

	if len(input.Entries) > 0 && len(input.Entries) != input.Size {
		c.JSON(http.StatusBadRequest, gin.H{"error": "entries must match bracket size"})
		return
	}

	bracket.Prepare()

	newBracket, err := bracket.SaveBracket(s.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create bracket"})
		return
	}

	generateFullBracket(s.DB, *newBracket, input.Entries)

	c.JSON(http.StatusCreated, gin.H{"response": bracketToDTO(s.DB, newBracket)})
}

//
// ===============================
// ADVANCE BRACKET
// ===============================
//

// AdvanceBracket godoc
// @Summary      Advance bracket
// @Description  Advance a bracket to the next round
// @Tags         brackets
// @Produce      json
// @Param        id   path      string  true  "Bracket ID"
// @Success      200  {object}  SimpleMessageResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /brackets/{id}/advance [post]
// @Security     BearerAuth
func (s *Server) AdvanceBracket(c *gin.Context) {
	userID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

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

	var bracket models.Bracket
	if err := s.DB.First(&bracket, bracketRecord.ID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
		return
	}

	if bracket.AuthorID != userID && !httpctx.IsAdminRequest(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authorized"})
		return
	}

	if bracket.Status != "active" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bracket is not active"})
		return
	}
	advanced, err := s.advanceBracketInternal(s.DB, &bracket)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	invalidateBracketSummaryCache(bracket.ID)

	c.JSON(http.StatusOK, gin.H{
		"message":  "Bracket advanced successfully",
		"round":    bracket.CurrentRound,
		"advanced": advanced,
	})
}

// POST /internal/brackets/:id/advance
func (s *Server) InternalAdvanceBracket(c *gin.Context) {
	if c.GetHeader("X-Internal-Key") != os.Getenv("API_SECRET") {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	s.AdvanceBracket(c)
}

// GetBracket godoc
// @Summary      Get a bracket
// @Description  Get bracket details by ID
// @Tags         brackets
// @Produce      json
// @Param        id   path      string  true  "Bracket ID"
// @Success      200  {object}  BracketResponseEnvelope
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /brackets/{id} [get]
func (s *Server) GetBracket(c *gin.Context) {
	var bracket models.Bracket
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
	found, err := bracket.FindBracketByID(s.DB, bracketRecord.ID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
		return
	}

	viewerID, hasViewer := optionalViewerID(c)
	allowed, reason, err := canViewUserContent(s.DB, viewerID, hasViewer, &found.Author, found.Visibility, httpctx.IsAdminRequest(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to check bracket visibility"})
		return
	}
	if !allowed {
		respondVisibilityDenied(c, reason)
		return
	}

	if found.Status == "active" &&
		found.AdvanceMode == "timer" &&
		found.RoundEndsAt != nil &&
		!time.Now().Before(*found.RoundEndsAt) {
		if _, err := s.advanceBracketInternal(s.DB, found); err != nil {
			log.Printf("auto advance bracket %d: %v", found.ID, err)
		} else if refreshed, err := bracket.FindBracketByID(s.DB, found.ID); err == nil {
			found = refreshed
		}
	}

	var likesCount int64
	s.DB.Model(&models.BracketLike{}).
		Where("bracket_id = ?", found.ID).
		Count(&likesCount)
	found.LikesCount = likesCount

	c.JSON(http.StatusOK, gin.H{"response": bracketToDTO(s.DB, found)})
}

// GetUserBrackets godoc
// @Summary      List a user's brackets
// @Description  Get all brackets created by a user
// @Tags         brackets
// @Produce      json
// @Param        id   path      string  true  "User ID"
// @Success      200  {object}  UserBracketsResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /users/{id}/brackets [get]
func (s *Server) GetUserBrackets(c *gin.Context) {
	owner, err := resolveUserByIdentifier(s.DB, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	viewerID, hasViewer := optionalViewerID(c)
	isAdmin := httpctx.IsAdminRequest(c)
	allowed, reason, err := canViewUserContent(s.DB, viewerID, hasViewer, owner, visibilityPublic, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to check visibility"})
		return
	}
	if !allowed {
		respondVisibilityDenied(c, reason)
		return
	}

	var bracket models.Bracket
	brackets, err := bracket.FindUserBrackets(s.DB, owner.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to retrieve brackets"})
		return
	}

	filtered := make([]BracketDTO, 0, len(*brackets))
	for i := range *brackets {
		allowed, _, err := canViewUserContent(s.DB, viewerID, hasViewer, owner, (*brackets)[i].Visibility, isAdmin)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to check bracket visibility"})
			return
		}
		if !allowed {
			continue
		}
		var likesCount int64
		s.DB.Model(&models.BracketLike{}).
			Where("bracket_id = ?", (*brackets)[i].ID).
			Count(&likesCount)
		(*brackets)[i].LikesCount = likesCount
		filtered = append(filtered, bracketToDTO(s.DB, &(*brackets)[i]))
	}

	c.JSON(http.StatusOK, gin.H{"response": filtered})
}

// UpdateBracket updates an existing bracket
// UpdateBracket godoc
// @Summary      Update a bracket
// @Description  Update bracket details
// @Tags         brackets
// @Accept       json
// @Produce      json
// @Param        id    path      string                  true  "Bracket ID"
// @Param        body  body      UpdateBracketRequest true  "Bracket payload"
// @Success      200   {object}  BracketResponseEnvelope
// @Failure      400   {object}  ErrorResponse
// @Failure      401   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Router       /brackets/{id} [put]
// @Security     BearerAuth
func (s *Server) UpdateBracket(c *gin.Context) {
	userID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

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

	var bracket models.Bracket
	if err := s.DB.First(&bracket, bracketRecord.ID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
		return
	}

	if bracket.AuthorID != userID && !httpctx.IsAdminRequest(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authorized"})
		return
	}

	var input struct {
		Title                *string `json:"title"`
		Description          *string `json:"description"`
		Status               *string `json:"status"`
		AdvanceMode          *string `json:"advance_mode"`
		DurationMinutes      *int    `json:"duration_minutes"`
		RoundDurationSeconds *int    `json:"round_duration_seconds"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	previousStatus := bracket.Status

	if input.Title != nil {
		bracket.Title = *input.Title
	}
	if input.Description != nil {
		bracket.Description = *input.Description
	}
	if input.Status != nil {
		bracket.Status = *input.Status
	}
	if input.AdvanceMode != nil {
		bracket.AdvanceMode = *input.AdvanceMode
	}
	if input.RoundDurationSeconds != nil {
		bracket.RoundDurationSeconds = *input.RoundDurationSeconds
	} else if input.DurationMinutes != nil {
		bracket.RoundDurationSeconds = *input.DurationMinutes * 60
	}

	activate := input.Status != nil && previousStatus != "active" && bracket.Status == "active"
	advanceModeChanged := input.AdvanceMode != nil
	if bracket.AdvanceMode != "timer" {
		bracket.RoundStartedAt = nil
		bracket.RoundEndsAt = nil
	} else if bracket.Status == "active" && (activate || (advanceModeChanged && bracket.RoundEndsAt == nil)) {
		setBracketRoundWindow(&bracket, time.Now())
	}

	bracket.Prepare()
	var updated *models.Bracket
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		updated, err = bracket.UpdateBracket(tx)
		if err != nil {
			return err
		}

		if activate || (advanceModeChanged && bracket.Status == "active") {
			if err := syncBracketMatchups(tx, &bracket); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update bracket"})
		return
	}
	invalidateBracketSummaryCache(bracket.ID)

	c.JSON(http.StatusOK, gin.H{"response": bracketToDTO(s.DB, updated)})
}

// DeleteBracket deletes a bracket
// DeleteBracket godoc
// @Summary      Delete a bracket
// @Description  Delete a bracket owned by the authenticated user
// @Tags         brackets
// @Produce      json
// @Param        id   path      string  true  "Bracket ID"
// @Success      200  {object}  SimpleMessageResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /brackets/{id} [delete]
// @Security     BearerAuth
func (s *Server) DeleteBracket(c *gin.Context) {
	userID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

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

	var bracket models.Bracket
	if err := s.DB.First(&bracket, bracketRecord.ID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
		return
	}

	if bracket.AuthorID != userID && !httpctx.IsAdminRequest(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authorized"})
		return
	}

	if err := s.deleteBracketCascade(&bracket); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete bracket"})
		return
	}
	invalidateBracketSummaryCache(bracket.ID)

	c.JSON(http.StatusOK, gin.H{"message": "Bracket deleted"})
}

func (s *Server) deleteBracketCascade(bracket *models.Bracket) error {
	matchups, err := models.FindMatchupsByBracket(s.DB, bracket.ID)
	if err != nil {
		return err
	}

	for i := range matchups {
		if err := s.deleteMatchupCascade(&matchups[i]); err != nil {
			return err
		}
	}

	if err := s.DB.Where("bracket_id = ?", bracket.ID).
		Delete(&models.BracketLike{}).Error; err != nil {
		return err
	}
	if err := s.DB.Where("bracket_id = ?", bracket.ID).
		Delete(&models.BracketComment{}).Error; err != nil {
		return err
	}

	if _, err := bracket.DeleteBracket(s.DB, bracket.ID); err != nil {
		return err
	}
	invalidateBracketSummaryCache(bracket.ID)
	invalidateHomeSummaryCache(bracket.AuthorID)

	return nil
}

// AttachMatchupToBracket godoc
// @Summary      Attach matchup to bracket
// @Description  Attach a matchup to a bracket round
// @Tags         brackets
// @Accept       json
// @Produce      json
// @Param        id    path      string                 true  "Bracket ID"
// @Param        body  body      AttachMatchupRequest true  "Attach payload"
// @Success      200   {object}  MatchupResponseEnvelope
// @Failure      400   {object}  ErrorResponse
// @Failure      401   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Router       /brackets/{id}/matchups [post]
// @Security     BearerAuth
func (s *Server) AttachMatchupToBracket(c *gin.Context) {
	// Auth
	userID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

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

	// Load bracket
	var bracket models.Bracket
	if err := s.DB.First(&bracket, bracketRecord.ID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
		return
	}

	// Ownership check
	if bracket.AuthorID != userID && !httpctx.IsAdminRequest(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authorized"})
		return
	}

	// 🔒 IMPORTANT: state check BEFORE modifying anything
	if bracket.Status != "draft" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "cannot modify bracket once active",
		})
		return
	}

	// Input
	var input struct {
		MatchupID uint `json:"matchup_id"`
		Round     int  `json:"round"`
		Seed      *int `json:"seed,omitempty"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.Round <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "round must be greater than 0"})
		return
	}

	// Load matchup
	var matchup models.Matchup
	if err := s.DB.First(&matchup, input.MatchupID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Matchup not found"})
		return
	}

	// Ownership consistency
	if matchup.AuthorID != userID && !httpctx.IsAdminRequest(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authorized to attach this matchup"})
		return
	}

	// Attach matchup to bracket
	matchup.BracketID = ptrUint(bracketRecord.ID)
	matchup.Round = ptrInt(input.Round)

	if input.Seed != nil {
		matchup.Seed = input.Seed
	}

	if err := s.DB.Save(&matchup).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to attach matchup"})
		return
	}
	invalidateBracketSummaryCache(bracket.ID)

	var reloaded models.Matchup
	if err := s.DB.Preload("Items").Preload("Author").First(&reloaded, matchup.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to load matchup"})
		return
	}
	var likesCount int64
	s.DB.Model(&models.Like{}).Where("matchup_id = ?", reloaded.ID).Count(&likesCount)

	c.JSON(http.StatusOK, gin.H{
		"response": toMatchupResponse(s.DB, &reloaded, []models.Comment{}, likesCount),
	})
}

// GetBracketMatchups godoc
// @Summary      List bracket matchups
// @Description  Get matchups for a bracket
// @Tags         brackets
// @Produce      json
// @Param        id   path      string  true  "Bracket ID"
// @Success      200  {object}  BracketMatchupsResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /brackets/{id}/matchups [get]
func (s *Server) GetBracketMatchups(c *gin.Context) {
	var bracket models.Bracket
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
	found, err := bracket.FindBracketByID(s.DB, bracketRecord.ID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
		return
	}

	viewerID, hasViewer := optionalViewerID(c)
	allowed, reason, err := canViewUserContent(s.DB, viewerID, hasViewer, &found.Author, found.Visibility, httpctx.IsAdminRequest(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to check bracket visibility"})
		return
	}
	if !allowed {
		respondVisibilityDenied(c, reason)
		return
	}

	if found.Status == "active" &&
		found.AdvanceMode == "timer" &&
		found.RoundEndsAt != nil &&
		!time.Now().Before(*found.RoundEndsAt) {
		if _, err := s.advanceBracketInternal(s.DB, found); err != nil {
			log.Printf("auto advance bracket %d: %v", found.ID, err)
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

	response := make([]MatchupDTO, 0, len(matchups))
	for i := range matchups {
		var likesCount int64
		s.DB.Model(&models.Like{}).Where("matchup_id = ?", matchups[i].ID).Count(&likesCount)
		response = append(response, toMatchupResponse(s.DB, &matchups[i], []models.Comment{}, likesCount))
	}

	c.JSON(http.StatusOK, gin.H{"response": response})
}

// DetachMatchupFromBracket godoc
// @Summary      Detach matchup from bracket
// @Description  Detach a matchup from a bracket
// @Tags         brackets
// @Produce      json
// @Param        matchup_id   path      string  true  "Matchup ID"
// @Success      200          {object}  SimpleMessageResponse
// @Failure      400          {object}  ErrorResponse
// @Failure      401          {object}  ErrorResponse
// @Failure      404          {object}  ErrorResponse
// @Router       /brackets/matchups/{matchup_id} [delete]
// @Security     BearerAuth
func (s *Server) DetachMatchupFromBracket(c *gin.Context) {
	userID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	matchupRecord, err := resolveMatchupByIdentifier(s.DB, c.Param("matchup_id"))
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid matchup ID"})
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Matchup not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Matchup not found"})
		}
		return
	}

	var matchup models.Matchup
	if err := s.DB.First(&matchup, matchupRecord.ID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Matchup not found"})
		return
	}

	if matchup.AuthorID != userID && !httpctx.IsAdminRequest(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authorized"})
		return
	}

	previousBracketID := matchup.BracketID
	matchup.BracketID = nil
	matchup.Round = nil

	if err := s.DB.Save(&matchup).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to detach matchup"})
		return
	}
	if previousBracketID != nil {
		invalidateBracketSummaryCache(*previousBracketID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Matchup detached"})
}

// advanceBracketInternal performs the core bracket advancement logic.
// It returns (advanced, error).
func (s *Server) advanceBracketInternal(
	tx *gorm.DB,
	bracket *models.Bracket,
) (bool, error) {
	advanced := false
	err := tx.Transaction(func(t *gorm.DB) error {
		var locked models.Bracket
		if err := t.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&locked, bracket.ID).Error; err != nil {
			return err
		}
		*bracket = locked

		currentRound := bracket.CurrentRound
		nextRound := currentRound + 1

		// Load all matchups for this bracket with items
		var matchups []models.Matchup
		if err := t.
			Preload("Items").
			Where("bracket_id = ?", bracket.ID).
			Order("round ASC, id ASC").
			Find(&matchups).Error; err != nil {
			return err
		}

		rounds := groupMatchupsByRound(matchups)

		currentMatchups := rounds[currentRound]
		if len(currentMatchups) == 0 {
			return fmt.Errorf("no matchups found for current round")
		}

		for i := range currentMatchups {
			if err := dedupeBracketMatchupItems(t, &currentMatchups[i]); err != nil {
				return err
			}
		}

		nextMatchups := rounds[nextRound]

		roundExpired := bracket.AdvanceMode == "timer" &&
			bracket.RoundEndsAt != nil &&
			!time.Now().Before(*bracket.RoundEndsAt)

		if roundExpired {
			now := time.Now()
			for i := range currentMatchups {
				matchup := &currentMatchups[i]
				if matchup.Status == matchupStatusCompleted && matchup.WinnerItemID != nil {
					continue
				}
				winnerID, err := determineMatchupWinnerByVotesOrSeed(matchup)
				if err != nil {
					return err
				}

				if err := t.Model(&models.Matchup{}).
					Where("id = ?", matchup.ID).
					Updates(map[string]interface{}{
						"winner_item_id": winnerID,
						"status":         matchupStatusCompleted,
						"updated_at":     now,
					}).Error; err != nil {
					return err
				}

				matchup.WinnerItemID = &winnerID
				matchup.Status = matchupStatusCompleted
			}
		}

		// Prevent advancing if next round already populated
		for _, nm := range nextMatchups {
			if len(nm.Items) > 0 {
				return fmt.Errorf("next round already populated")
			}
		}

		// ----------------------------------------
		// DETERMINE WINNERS
		// ----------------------------------------
		winners := make([]string, 0, len(currentMatchups))
		for _, m := range currentMatchups {
			if m.Status != matchupStatusCompleted {
				return fmt.Errorf("matchup %d not completed", m.ID)
			}

			winner, err := m.WinnerItem()
			if err != nil {
				return err
			}

			winners = append(winners, winner.Item)
		}

		// ----------------------------------------
		// FINAL ROUND → COMPLETE BRACKET
		// ----------------------------------------
		if len(nextMatchups) == 0 {
			bracket.Status = "completed"
			now := time.Now()
			bracket.CompletedAt = &now
			if _, err := bracket.UpdateBracket(t); err != nil {
				return err
			}
			advanced = true
			return nil
		}

		if len(winners) != len(nextMatchups)*2 {
			return fmt.Errorf(
				"invalid bracket state: expected %d winners, got %d",
				len(nextMatchups)*2,
				len(winners),
			)
		}

		// ----------------------------------------
		// POPULATE + ADVANCE + UNLOCK
		// ----------------------------------------
		w := 0
		for i := range nextMatchups {
			nm := &nextMatchups[i]

			if err := t.Where("matchup_id = ?", nm.ID).
				Delete(&models.MatchupItem{}).Error; err != nil {
				return err
			}

			items := []models.MatchupItem{
				{MatchupID: nm.ID, Item: winners[w]},
				{MatchupID: nm.ID, Item: winners[w+1]},
			}
			w += 2

			if err := t.Create(&items).Error; err != nil {
				return err
			}
		}

		// 🔁 Advance round
		bracket.CurrentRound = nextRound
		setBracketRoundWindow(bracket, time.Now())
		if _, err := bracket.UpdateBracket(t); err != nil {
			return err
		}

		if err := syncBracketMatchups(t, bracket); err != nil {
			return err
		}

		advanced = true
		return nil
	})
	if err != nil {
		return false, err
	}

	return advanced, nil
}
