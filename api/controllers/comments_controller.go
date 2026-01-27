package controllers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"Matchup/auth"
	"Matchup/models"
	"Matchup/utils/formaterror"
	httpctx "Matchup/utils/httpctx"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type CommentUser struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
}

// CreateComment godoc
// @Summary      Create a matchup comment
// @Description  Add a comment to a matchup
// @Tags         comments
// @Accept       json
// @Produce      json
// @Param        id       path      string                  true  "Matchup ID"
// @Param        comment  body      CommentCreateRequest true  "Comment payload"
// @Success      201      {object}  CommentResponseDoc
// @Failure      400      {object}  ErrorResponse
// @Failure      401      {object}  ErrorResponse
// @Failure      403      {object}  ErrorResponse
// @Failure      422      {object}  ErrorResponse
// @Router       /matchups/{id}/comments [post]
// @Security     BearerAuth
func (server *Server) CreateComment(c *gin.Context) {
	errList := map[string]string{}

	// Parse matchup ID from the URL
	matchupID := c.Param("id")
	matchupRecord, err := resolveMatchupByIdentifier(server.DB, matchupID)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			errList["Invalid_request"] = "Invalid Request"
			c.JSON(http.StatusBadRequest, gin.H{
				"status": http.StatusBadRequest,
				"error":  errList,
			})
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			errList["No_matchup"] = "No matchup found"
			c.JSON(http.StatusNotFound, gin.H{
				"status": http.StatusNotFound,
				"error":  errList,
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to retrieve matchup"})
		}
		return
	}

	// Extract the user ID from the token
	uid, err := auth.ExtractTokenID(c.Request)
	if err != nil {
		errList["Unauthorized"] = "Unauthorized"
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": http.StatusUnauthorized,
			"error":  errList,
		})
		return
	}

	// Verify the user exists
	user := models.User{}
	err = server.DB.Where("id = ?", uid).Take(&user).Error
	if err != nil {
		errList["Unauthorized"] = "User not found"
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": http.StatusUnauthorized,
			"error":  errList,
		})
		return
	}

	// Verify the matchup exists
	matchup := models.Matchup{}
	err = server.DB.Preload("Author").Where("id = ?", matchupRecord.ID).Take(&matchup).Error
	if err != nil {
		errList["No_matchup"] = "No matchup found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	allowed, reason, err := canViewUserContent(server.DB, uid, true, &matchup.Author, matchup.Visibility, httpctx.IsAdminRequest(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to check matchup visibility"})
		return
	}
	if !allowed {
		respondVisibilityDenied(c, reason)
		return
	}

	if matchup.Status == matchupStatusDraft {
		c.JSON(http.StatusForbidden, gin.H{"error": "Matchup is not active"})
		return
	}

	if matchup.BracketID != nil && matchup.Status != matchupStatusCompleted {
		var bracket models.Bracket
		if err := server.DB.First(&bracket, *matchup.BracketID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Bracket not found"})
			return
		}

		if bracket.Status != "active" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Bracket is not active"})
			return
		}

		if matchup.Round == nil || *matchup.Round != bracket.CurrentRound {
			c.JSON(http.StatusForbidden, gin.H{"error": "Matchup is not in the active round"})
			return
		}
	}

	// Bind JSON to comment struct
	var comment models.Comment
	if err := c.ShouldBindJSON(&comment); err != nil {
		errList["Invalid_body"] = "Invalid request body"
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
			"error":  errList,
		})
		return
	}

	// Set the user ID and matchup ID
	comment.UserID = uid
	comment.MatchupID = matchupRecord.ID

	// Prepare and validate the comment
	comment.Prepare()
	errorMessages := comment.Validate("")
	if len(errorMessages) > 0 {
		errList = errorMessages
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
			"error":  errList,
		})
		return
	}

	// Save the comment
	commentCreated, err := comment.SaveComment(server.DB)
	if err != nil {
		formattedError := formaterror.FormatError(err.Error())
		errList = formattedError
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  errList,
		})
		return
	}

	// Prepare the response with the username
	commentResponse := CommentDTO{
		ID:        commentCreated.PublicID,
		UserID:    user.PublicID,
		Username:  user.Username, // Use the username of the authenticated user
		Body:      commentCreated.Body,
		CreatedAt: commentCreated.CreatedAt,
		UpdatedAt: commentCreated.UpdatedAt,
	}

	// Send the response
	c.JSON(http.StatusCreated, gin.H{
		"status":   http.StatusCreated,
		"response": commentResponse,
	})
	invalidateHomeSummaryCache(matchup.AuthorID)
}

// GetComments godoc
// @Summary      List matchup comments
// @Description  Get comments for a matchup
// @Tags         comments
// @Produce      json
// @Param        id   path      string  true  "Matchup ID"
// @Success      200  {object}  CommentListResponseDoc
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /matchups/{id}/comments [get]
func (server *Server) GetComments(c *gin.Context) {
	errList := map[string]string{}

	// Parse the matchup ID from the URL parameters
	matchupID := c.Param("id")
	matchupRecord, err := resolveMatchupByIdentifier(server.DB, matchupID)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			errList["Invalid_request"] = "Invalid Request"
			c.JSON(http.StatusBadRequest, gin.H{
				"status": http.StatusBadRequest,
				"error":  errList,
			})
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			errList["No_matchup"] = "No matchup found"
			c.JSON(http.StatusNotFound, gin.H{
				"status": http.StatusNotFound,
				"error":  errList,
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to retrieve matchup"})
		}
		return
	}

	// Check if the matchup exists
	matchup := models.Matchup{}
	err = server.DB.Preload("Author").Where("id = ?", matchupRecord.ID).Take(&matchup).Error
	if err != nil {
		errList["No_matchup"] = "No matchup found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	viewerID, hasViewer := optionalViewerID(c)
	allowed, reason, err := canViewUserContent(server.DB, viewerID, hasViewer, &matchup.Author, matchup.Visibility, httpctx.IsAdminRequest(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to check matchup visibility"})
		return
	}
	if !allowed {
		respondVisibilityDenied(c, reason)
		return
	}

	// Load comments with the associated author's username using Preload
	comments := []models.Comment{}
	err = server.DB.Preload("Author").Where("matchup_id = ?", matchupRecord.ID).Order("created_at desc").Find(&comments).Error
	if err != nil {
		errList["No_comments"] = "No comments found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	// Prepare response data with comments including usernames
	commentResponses := []CommentDTO{}
	for _, comment := range comments {
		commentResponses = append(commentResponses, commentToDTO(server.DB, comment))
	}

	// Send the response back to the client
	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": commentResponses,
	})
}

// UpdateComment godoc
// @Summary      Update a comment
// @Description  Update a matchup comment (owner only)
// @Tags         comments
// @Accept       json
// @Produce      json
// @Param        id      path      string                  true  "Comment ID"
// @Param        comment body      CommentUpdateRequest true  "Comment payload"
// @Success      200     {object}  CommentResponseDoc
// @Failure      400     {object}  ErrorResponse
// @Failure      401     {object}  ErrorResponse
// @Failure      404     {object}  ErrorResponse
// @Failure      422     {object}  ErrorResponse
// @Router       /comments/{id} [put]
// @Security     BearerAuth
func (server *Server) UpdateComment(c *gin.Context) {
	errList := map[string]string{}

	commentID := c.Param("id")
	commentRecord, err := resolveCommentByIdentifier(server.DB, commentID)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			errList["Invalid_request"] = "Invalid Request"
			c.JSON(http.StatusBadRequest, gin.H{
				"status": http.StatusBadRequest,
				"error":  errList,
			})
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			errList["No_comment"] = "No Comment Found"
			c.JSON(http.StatusNotFound, gin.H{
				"status": http.StatusNotFound,
				"error":  errList,
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error finding comment"})
		}
		return
	}

	uid, err := auth.ExtractTokenID(c.Request)
	if err != nil {
		errList["Unauthorized"] = "Unauthorized"
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": http.StatusUnauthorized,
			"error":  errList,
		})
		return
	}

	origComment := models.Comment{}
	err = server.DB.Model(models.Comment{}).Where("id = ?", commentRecord.ID).Take(&origComment).Error
	if err != nil {
		errList["No_comment"] = "No Comment Found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}
	if uid != origComment.UserID && !httpctx.IsAdminRequest(c) {
		errList["Unauthorized"] = "Unauthorized"
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": http.StatusUnauthorized,
			"error":  errList,
		})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		errList["Invalid_body"] = "Unable to get request"
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
			"error":  errList,
		})
		return
	}

	comment := models.Comment{}
	err = json.Unmarshal(body, &comment)
	if err != nil {
		errList["Unmarshal_error"] = "Cannot unmarshal body"
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
			"error":  errList,
		})
		return
	}

	comment.Prepare()
	errorMessages := comment.Validate("")
	if len(errorMessages) > 0 {
		errList = errorMessages
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
			"error":  errList,
		})
		return
	}

	comment.ID = origComment.ID
	comment.UserID = origComment.UserID
	comment.MatchupID = origComment.MatchupID

	commentUpdated, err := comment.UpdateAComment(server.DB)
	if err != nil {
		formattedError := formaterror.FormatError(err.Error())
		errList = formattedError
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  errList,
		})
		return
	}

	var username string
	var userPublicID string
	var author models.User
	if err := server.DB.Select("username", "public_id").First(&author, commentUpdated.UserID).Error; err == nil {
		username = author.Username
		userPublicID = author.PublicID
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": CommentDTO{
			ID:        commentUpdated.PublicID,
			UserID:    userPublicID,
			Username:  username,
			Body:      commentUpdated.Body,
			CreatedAt: commentUpdated.CreatedAt,
			UpdatedAt: commentUpdated.UpdatedAt,
		},
	})
}

// DeleteComment godoc
// @Summary      Delete a comment
// @Description  Delete a matchup comment (owner only)
// @Tags         comments
// @Produce      json
// @Param        id   path      string  true  "Comment ID"
// @Success      200  {object}  SimpleMessageResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /comments/{id} [delete]
// @Security     BearerAuth
func (server *Server) DeleteComment(c *gin.Context) {
	errList := map[string]string{}

	commentID := c.Param("id")
	commentRecord, err := resolveCommentByIdentifier(server.DB, commentID)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			errList["Invalid_request"] = "Invalid Request"
			c.JSON(http.StatusBadRequest, gin.H{
				"status": http.StatusBadRequest,
				"error":  errList,
			})
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			errList["No_comment"] = "No Comment Found"
			c.JSON(http.StatusNotFound, gin.H{
				"status": http.StatusNotFound,
				"error":  errList,
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error finding comment"})
		}
		return
	}

	uid, err := auth.ExtractTokenID(c.Request)
	if err != nil {
		errList["Unauthorized"] = "Unauthorized"
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": http.StatusUnauthorized,
			"error":  errList,
		})
		return
	}

	comment := models.Comment{}
	err = server.DB.Debug().Model(models.Comment{}).Where("id = ?", commentRecord.ID).Take(&comment).Error
	if err != nil {
		errList["No_comment"] = "No Comment Found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	if uid != comment.UserID && !httpctx.IsAdminRequest(c) {
		errList["Unauthorized"] = "Unauthorized"
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": http.StatusUnauthorized,
			"error":  errList,
		})
		return
	}

	_, err = comment.DeleteAComment(server.DB)
	if err != nil {
		errList["Other_error"] = "Please try again later"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}
	var matchup models.Matchup
	if err := server.DB.First(&matchup, comment.MatchupID).Error; err == nil {
		invalidateHomeSummaryCache(matchup.AuthorID)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": "Comment deleted",
	})
}
