package controllers

import (
	"errors"
	"net/http"

	"Matchup/auth"
	"Matchup/models"
	"Matchup/utils/formaterror"
	httpctx "Matchup/utils/httpctx"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CreateBracketComment godoc
// @Summary      Create a bracket comment
// @Description  Add a comment to a bracket
// @Tags         bracket-comments
// @Accept       json
// @Produce      json
// @Param        id       path      string                        true  "Bracket ID"
// @Param        comment  body      BracketCommentCreateRequest true  "Comment payload"
// @Success      201      {object}  BracketCommentResponseDoc
// @Failure      400      {object}  ErrorResponse
// @Failure      401      {object}  ErrorResponse
// @Failure      403      {object}  ErrorResponse
// @Failure      422      {object}  ErrorResponse
// @Router       /brackets/{id}/comments [post]
// @Security     BearerAuth
func (server *Server) CreateBracketComment(c *gin.Context) {
	errList := map[string]string{}

	bracketID := c.Param("id")
	bracketRecord, err := resolveBracketByIdentifier(server.DB, bracketID)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			errList["Invalid_request"] = "Invalid Request"
			c.JSON(http.StatusBadRequest, gin.H{
				"status": http.StatusBadRequest,
				"error":  errList,
			})
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			errList["No_bracket"] = "No bracket found"
			c.JSON(http.StatusNotFound, gin.H{
				"status": http.StatusNotFound,
				"error":  errList,
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to retrieve bracket"})
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
	if err := server.DB.Preload("Author").Where("id = ?", bracketRecord.ID).Take(&bracket).Error; err != nil {
		errList["No_bracket"] = "No bracket found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	allowed, reason, err := canViewUserContent(server.DB, uid, true, &bracket.Author, bracket.Visibility, httpctx.IsAdminRequest(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to check bracket visibility"})
		return
	}
	if !allowed {
		respondVisibilityDenied(c, reason)
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
	comment.BracketID = bracketRecord.ID

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

	commentResponse := CommentDTO{
		ID:        commentCreated.PublicID,
		UserID:    user.PublicID,
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

// GetBracketComments godoc
// @Summary      List bracket comments
// @Description  Get comments for a bracket
// @Tags         bracket-comments
// @Produce      json
// @Param        id   path      string  true  "Bracket ID"
// @Success      200  {object}  BracketCommentListResponseDoc
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /brackets/{id}/comments [get]
func (server *Server) GetBracketComments(c *gin.Context) {
	errList := map[string]string{}

	bracketID := c.Param("id")
	bracketRecord, err := resolveBracketByIdentifier(server.DB, bracketID)
	if err != nil {
		if errors.Is(err, errInvalidIdentifier) {
			errList["Invalid_request"] = "Invalid Request"
			c.JSON(http.StatusBadRequest, gin.H{
				"status": http.StatusBadRequest,
				"error":  errList,
			})
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			errList["No_bracket"] = "No bracket found"
			c.JSON(http.StatusNotFound, gin.H{
				"status": http.StatusNotFound,
				"error":  errList,
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to retrieve bracket"})
		}
		return
	}

	bracket := models.Bracket{}
	if err := server.DB.Preload("Author").Where("id = ?", bracketRecord.ID).Take(&bracket).Error; err != nil {
		errList["No_bracket"] = "No bracket found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	viewerID, hasViewer := optionalViewerID(c)
	allowed, reason, err := canViewUserContent(server.DB, viewerID, hasViewer, &bracket.Author, bracket.Visibility, httpctx.IsAdminRequest(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to check bracket visibility"})
		return
	}
	if !allowed {
		respondVisibilityDenied(c, reason)
		return
	}

	comments := []models.BracketComment{}
	if err := server.DB.Preload("Author").Where("bracket_id = ?", bracketRecord.ID).
		Order("created_at desc").Find(&comments).Error; err != nil {
		errList["No_comments"] = "No comments found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	commentResponses := []CommentDTO{}
	for _, comment := range comments {
		commentResponses = append(commentResponses, bracketCommentToDTO(server.DB, comment))
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": commentResponses,
	})
}

// DeleteBracketComment godoc
// @Summary      Delete a bracket comment
// @Description  Delete a bracket comment (owner or admin)
// @Tags         bracket-comments
// @Produce      json
// @Param        id   path      string  true  "Comment ID"
// @Success      200  {object}  SimpleMessageResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /bracket_comments/{id} [delete]
// @Security     BearerAuth
func (server *Server) DeleteBracketComment(c *gin.Context) {
	errList := map[string]string{}

	commentID := c.Param("id")
	commentRecord, err := resolveBracketCommentByIdentifier(server.DB, commentID)
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
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": http.StatusInternalServerError,
				"error":  "Error finding comment",
			})
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

	comment := models.BracketComment{}
	if err := server.DB.Model(models.BracketComment{}).Where("id = ?", commentRecord.ID).Take(&comment).Error; err != nil {
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
