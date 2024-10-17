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
	"github.com/google/uuid"
)

type CommentResponse struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Username  string    `json:"username"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Nested User struct with only required fields
type CommentUser struct {
	ID       uuid.UUID `json:"id"`
	Username string    `json:"username"`
}

func ExtractUserIDsFromComments(comments []CommentResponse) []uuid.UUID {
	userIDs := make([]uuid.UUID, len(comments))
	for i, comment := range comments {
		userIDs[i] = comment.UserID
	}
	return userIDs
}

func (server *Server) CreateComment(c *gin.Context) {
	// Clear previous error if any
	errList := map[string]string{}

<<<<<<< Updated upstream
	matchupID := c.Param("id")
	mid, err := uuid.Parse(matchupID)
=======
<<<<<<< HEAD
	matchupID := c.Param("matchup_id")
	mid, err := strconv.ParseUint(matchupID, 10, 32)
=======
	matchupID := c.Param("id")
	mid, err := uuid.Parse(matchupID)
>>>>>>> golang-version
>>>>>>> Stashed changes
	if err != nil {
		errList["Invalid_request"] = "Invalid Request"
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  errList,
		})
		return
	}

	// Check the token
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
	err = server.DB.Model(models.Matchup{}).Where("id = ?", mid).Take(&matchup).Error
	if err != nil {
		errList["Unauthorized"] = "Unauthorized"
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": http.StatusUnauthorized,
			"error":  errList,
		})
		return
	}

	// Read the request body
	var comment models.Comment
	if err := c.ShouldBindJSON(&comment); err != nil {
		errList["Invalid_body"] = "Invalid request body"
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
			"error":  errList,
		})
		return
	}

	// Set the UserID and MatchupID for the comment
	comment.UserID = uid
	comment.MatchupID = mid

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
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	// Create the CommentResponse object
	commentResponse := CommentResponse{
		ID:        commentCreated.ID,
		UserID:    commentCreated.UserID,
		Username:  user.Username,
		Body:      commentCreated.Body,
		CreatedAt: commentCreated.CreatedAt,
	}

	// Return the CommentResponse in the JSON response
	c.JSON(http.StatusCreated, gin.H{
		"status":   http.StatusCreated,
		"response": commentResponse,
	})
}

func (server *Server) GetComments(c *gin.Context) {
	errList := map[string]string{}

	matchupID := c.Param("id")

	// Is a valid matchup ID given to us?
	mid, err := uuid.Parse(matchupID)
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
	err = server.DB.Model(models.Matchup{}).Where("id = ?", mid).Take(&matchup).Error
	if err != nil {
		errList["No_matchup"] = "No matchup found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	// Get the comments associated with the matchup
	comments := []CommentResponse{}
	err = server.DB.Model(models.Comment{}).
		Select("id, user_id, body, created_at, updated_at").
		Where("matchup_id = ?", mid).
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

	// Fetch the associated users
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

	// Map user information to comments
	userMap := make(map[uuid.UUID]CommentUser)
	for _, user := range users {
		userMap[user.ID] = user
	}

	// Populate the username field in comments
	for i, comment := range comments {
		comments[i].Username = userMap[comment.UserID].Username
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": comments,
	})
}

func (server *Server) UpdateComment(c *gin.Context) {

	//clear previous error if any
	errList = map[string]string{}

	commentID := c.Param("id")
	// Check if the comment id is valid
	cid, err := strconv.ParseUint(commentID, 10, 64)
	if err != nil {
		errList["Invalid_request"] = "Invalid Request"
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  errList,
		})
		return
	}
	//CHeck if the auth token is valid and  get the user id from it
	uid, err := auth.ExtractTokenID(c.Request)
	if err != nil {
		errList["Unauthorized"] = "Unauthorized"
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": http.StatusUnauthorized,
			"error":  errList,
		})
		return
	}
	//Check if the comment exist
	origComment := models.Comment{}
	err = server.DB.Model(models.Comment{}).Where("id = ?", cid).Take(&origComment).Error
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
	// Read the data posted
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		errList["Invalid_body"] = "Unable to get request"
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
			"error":  errList,
		})
		return
	}
	// Start processing the request data
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
	comment.ID = origComment.ID //this is important to tell the model the matchup id to update, the other update field are set above
	comment.UserID = origComment.UserID
	comment.MatchupID = origComment.MatchupID

	commentUpdated, err := comment.UpdateAComment(server.DB)
	if err != nil {
		formattedError := formaterror.FormatError(err.Error())
		errList = formattedError
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  err,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": commentUpdated,
	})
}

func (server *Server) DeleteComment(c *gin.Context) {

	commentID := c.Param("id")
	// Is a valid matchup id given to us?
	cid, err := strconv.ParseUint(commentID, 10, 64)
	if err != nil {
		errList["Invalid_request"] = "Invalid Request"
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  errList,
		})
		return
	}
	// Is this user authenticated?
	uid, err := auth.ExtractTokenID(c.Request)
	if err != nil {
		errList["Unauthorized"] = "Unauthorized"
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": http.StatusUnauthorized,
			"error":  errList,
		})
		return
	}
	// Check if the comment exist
	comment := models.Comment{}
	err = server.DB.Debug().Model(models.Comment{}).Where("id = ?", cid).Take(&comment).Error
	if err != nil {
		errList["No_matchup"] = "No Matchup Found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}
	// Is the authenticated user, the owner of this matchup?
	if uid != comment.UserID {
		errList["Unauthorized"] = "Unauthorized"
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": http.StatusUnauthorized,
			"error":  errList,
		})
		return
	}

	// If all the conditions are met, delete the matchup
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
