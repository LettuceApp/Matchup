package controllers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"Matchup/auth"
	"Matchup/models"
	"Matchup/utils/formaterror"
	httpctx "Matchup/utils/httpctx"

	"github.com/gin-gonic/gin"
)

type CommentResponse struct {
	ID        uint      `json:"id"`
	UserID    uint      `json:"user_id"`
	Username  string    `json:"username"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CommentUser struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
}

func ExtractUserIDsFromComments(comments []CommentResponse) []uint {
	userIDs := make([]uint, len(comments))
	for i, comment := range comments {
		userIDs[i] = comment.UserID
	}
	return userIDs
}

func (server *Server) CreateComment(c *gin.Context) {
	errList := map[string]string{}

	// Parse matchup ID from the URL
	matchupID := c.Param("id")
	mid, err := strconv.ParseUint(matchupID, 10, 32)
	if err != nil {
		errList["Invalid_request"] = "Invalid Request"
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  errList,
		})
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
	err = server.DB.Where("id = ?", uint(mid)).Take(&matchup).Error
	if err != nil {
		errList["No_matchup"] = "No matchup found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	if !isMatchupOpenStatus(matchup.Status) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Matchup is not active"})
		return
	}

	if matchup.EndTime != nil && time.Now().After(*matchup.EndTime) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Matchup has ended"})
		return
	}

	if matchup.BracketID != nil {
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
	comment.MatchupID = uint(mid)

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
	commentResponse := CommentResponse{
		ID:        commentCreated.ID,
		UserID:    commentCreated.UserID,
		Username:  user.Username, // Use the username of the authenticated user
		Body:      commentCreated.Body,
		CreatedAt: commentCreated.CreatedAt,
	}

	// Send the response
	c.JSON(http.StatusCreated, gin.H{
		"status":   http.StatusCreated,
		"response": commentResponse,
	})
	invalidateHomeSummaryCache(matchup.AuthorID)
}

func (server *Server) GetComments(c *gin.Context) {
	errList := map[string]string{}

	// Parse the matchup ID from the URL parameters
	matchupID := c.Param("id")
	mid, err := strconv.ParseUint(matchupID, 10, 32)
	if err != nil {
		errList["Invalid_request"] = "Invalid Request"
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  errList,
		})
		return
	}

	// Check if the matchup exists
	matchup := models.Matchup{}
	err = server.DB.Model(models.Matchup{}).Where("id = ?", uint(mid)).Take(&matchup).Error
	if err != nil {
		errList["No_matchup"] = "No matchup found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	// Load comments with the associated author's username using Preload
	comments := []models.Comment{}
	err = server.DB.Preload("Author").Where("matchup_id = ?", uint(mid)).Order("created_at desc").Find(&comments).Error
	if err != nil {
		errList["No_comments"] = "No comments found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	// Prepare response data with comments including usernames
	commentResponses := []map[string]interface{}{}
	for _, comment := range comments {
		commentResponses = append(commentResponses, map[string]interface{}{
			"id":         comment.ID,
			"user_id":    comment.UserID,
			"username":   comment.Author.Username, // Use the Author's username
			"body":       comment.Body,
			"created_at": comment.CreatedAt,
			"updated_at": comment.UpdatedAt,
		})
	}

	// Send the response back to the client
	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": commentResponses,
	})
}

func (server *Server) UpdateComment(c *gin.Context) {
	errList := map[string]string{}

	commentID := c.Param("id")
	cid, err := strconv.ParseUint(commentID, 10, 32)
	if err != nil {
		errList["Invalid_request"] = "Invalid Request"
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  errList,
		})
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
	err = server.DB.Model(models.Comment{}).Where("id = ?", uint(cid)).Take(&origComment).Error
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

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": commentUpdated,
	})
}

func (server *Server) DeleteComment(c *gin.Context) {
	errList := map[string]string{}

	commentID := c.Param("id")
	cid, err := strconv.ParseUint(commentID, 10, 32)
	if err != nil {
		errList["Invalid_request"] = "Invalid Request"
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  errList,
		})
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
	err = server.DB.Debug().Model(models.Comment{}).Where("id = ?", uint(cid)).Take(&comment).Error
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
