package controllers

import (
	"net/http"
	"strconv"
	"time"

	"Matchup/auth"
	"Matchup/models"
	"Matchup/utils/formaterror"
	httpctx "Matchup/utils/httpctx"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type BracketCommentResponse struct {
	ID        uint      `json:"id"`
	UserID    uint      `json:"user_id"`
	Username  string    `json:"username"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (server *Server) CreateBracketComment(c *gin.Context) {
	errList := map[string]string{}

	bracketID := c.Param("id")
	bid, err := strconv.ParseUint(bracketID, 10, 32)
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

	user := models.User{}
	if err := server.DB.Where("id = ?", uid).Take(&user).Error; err != nil {
		errList["Unauthorized"] = "User not found"
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": http.StatusUnauthorized,
			"error":  errList,
		})
		return
	}

	bracket := models.Bracket{}
	if err := server.DB.Where("id = ?", uint(bid)).Take(&bracket).Error; err != nil {
		errList["No_bracket"] = "No bracket found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}
	if bracket.Status == "completed" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bracket comments are closed"})
		return
	}

	var comment models.BracketComment
	if err := c.ShouldBindJSON(&comment); err != nil {
		errList["Invalid_body"] = "Invalid request body"
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
			"error":  errList,
		})
		return
	}

	comment.UserID = uid
	comment.BracketID = uint(bid)

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
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  errList,
		})
		return
	}

	commentResponse := BracketCommentResponse{
		ID:        commentCreated.ID,
		UserID:    commentCreated.UserID,
		Username:  user.Username,
		Body:      commentCreated.Body,
		CreatedAt: commentCreated.CreatedAt,
		UpdatedAt: commentCreated.UpdatedAt,
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":   http.StatusCreated,
		"response": commentResponse,
	})
	invalidateHomeSummaryCache(bracket.AuthorID)
}

func (server *Server) GetBracketComments(c *gin.Context) {
	errList := map[string]string{}

	bracketID := c.Param("id")
	bid, err := strconv.ParseUint(bracketID, 10, 32)
	if err != nil {
		errList["Invalid_request"] = "Invalid Request"
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  errList,
		})
		return
	}

	bracket := models.Bracket{}
	if err := server.DB.Model(models.Bracket{}).Where("id = ?", uint(bid)).Take(&bracket).Error; err != nil {
		errList["No_bracket"] = "No bracket found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	comments := []models.BracketComment{}
	if err := server.DB.Preload("Author").Where("bracket_id = ?", uint(bid)).
		Order("created_at desc").Find(&comments).Error; err != nil {
		errList["No_comments"] = "No comments found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	commentResponses := []map[string]interface{}{}
	for _, comment := range comments {
		commentResponses = append(commentResponses, map[string]interface{}{
			"id":         comment.ID,
			"user_id":    comment.UserID,
			"username":   comment.Author.Username,
			"body":       comment.Body,
			"created_at": comment.CreatedAt,
			"updated_at": comment.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": commentResponses,
	})
}

func (server *Server) DeleteBracketComment(c *gin.Context) {
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

	comment := models.BracketComment{}
	if err := server.DB.Model(models.BracketComment{}).Where("id = ?", uint(cid)).Take(&comment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			errList["No_comment"] = "No Comment Found"
			c.JSON(http.StatusNotFound, gin.H{
				"status": http.StatusNotFound,
				"error":  errList,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  "Error finding comment",
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

	if _, err := comment.DeleteAComment(server.DB); err != nil {
		errList["Other_error"] = "Please try again later"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}
	var bracket models.Bracket
	if err := server.DB.First(&bracket, comment.BracketID).Error; err == nil {
		invalidateHomeSummaryCache(bracket.AuthorID)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": "Comment deleted",
	})
}
