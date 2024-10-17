package controllers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"Matchup/api/auth"
	"Matchup/api/models"
	"Matchup/api/utils/formaterror"

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

	matchupID := c.Param("matchup_id")
	mid, err := strconv.ParseUint(matchupID, 10, 32)
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

	// Check if the user exists
	user := models.User{}
	err = server.DB.Model(models.User{}).Where("id = ?", uid).Take(&user).Error
	if err != nil {
		errList["Unauthorized"] = "Unauthorized"
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": http.StatusUnauthorized,
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

	var comment models.Comment
	if err := c.ShouldBindJSON(&comment); err != nil {
		errList["Invalid_body"] = "Invalid request body"
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
			"error":  errList,
		})
		return
	}

	comment.UserID = uid
	comment.MatchupID = uint(mid)

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

	commentCreated, err := comment.SaveComment(server.DB)
	if err != nil {
		formattedError := formaterror.FormatError(err.Error())
		errList = formattedError
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	commentResponse := CommentResponse{
		ID:        commentCreated.ID,
		UserID:    commentCreated.UserID,
		Username:  user.Username,
		Body:      commentCreated.Body,
		CreatedAt: commentCreated.CreatedAt,
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":   http.StatusCreated,
		"response": commentResponse,
	})
}

func (server *Server) GetComments(c *gin.Context) {
	errList := map[string]string{}

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

	comments := []CommentResponse{}
	err = server.DB.Model(models.Comment{}).
		Select("id, user_id, body, created_at, updated_at").
		Where("matchup_id = ?", uint(mid)).
		Find(&comments).
		Error
	if err != nil {
		errList["No_comments"] = "No comments found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	users := []CommentUser{}
	err = server.DB.Model(models.User{}).
		Select("id, username").
		Where("id IN ?", ExtractUserIDsFromComments(comments)).
		Find(&users).
		Error
	if err != nil {
		errList["No_users"] = "No users found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	userMap := make(map[uint]CommentUser)
	for _, user := range users {
		userMap[user.ID] = user
	}

	for i, comment := range comments {
		comments[i].Username = userMap[comment.UserID].Username
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": comments,
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
	if uid != origComment.UserID {
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

	if uid != comment.UserID {
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

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": "Comment deleted",
	})
}
