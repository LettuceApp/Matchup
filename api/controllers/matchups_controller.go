package controllers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"Matchup/api/auth"
	"Matchup/api/models"
	"Matchup/api/utils/formaterror"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type MatchupResponse struct {
	ID        uuid.UUID             `json:"id"`
	Title     string                `json:"title"`
	Content   string                `json:"content"`
	AuthorID  uuid.UUID             `json:"author_id"`
	Items     []MatchupItemResponse `json:"items"`
	Comments  []CommentResponse     `json:"comments"`
	CreatedAt time.Time             `json:"created_at"`
	UpdatedAt time.Time             `json:"updated_at"`
}

type MatchupItemResponse struct {
	ID    uuid.UUID `json:"id"`
	Item  string    `json:"item"`
	Votes int       `json:"votes"`
}

func toMatchupResponse(matchup *models.Matchup, comments []models.Comment) *MatchupResponse {
	itemResponses := make([]MatchupItemResponse, len(matchup.Items))
	for i, item := range matchup.Items {
		itemResponses[i] = MatchupItemResponse{
			ID:    item.ID,
			Item:  item.Item,
			Votes: item.Votes,
		}
	}

	commentResponses := make([]CommentResponse, len(comments))
	for i, comment := range comments {
		commentResponses[i] = CommentResponse{
			ID:        comment.ID,
			UserID:    comment.UserID,
			Username:  comment.User.Username, // Assuming you have a User struct with Username field in the Comment model.
			Body:      comment.Body,
			CreatedAt: comment.CreatedAt,
			UpdatedAt: comment.UpdatedAt,
		}
	}

	return &MatchupResponse{
		ID:        matchup.ID,
		Title:     matchup.Title,
		Content:   matchup.Content,
		AuthorID:  matchup.AuthorID,
		Items:     itemResponses,
		Comments:  commentResponses,
		CreatedAt: matchup.CreatedAt,
		UpdatedAt: matchup.UpdatedAt,
	}
}

func (server *Server) CreateMatchup(c *gin.Context) {

	// clear previous error if any
	errList = map[string]string{}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		errList["Invalid_body"] = "Unable to get request"
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
			"error":  errList,
		})
		return
	}
	matchup := models.Matchup{}

	err = json.Unmarshal(body, &matchup)
	if err != nil {
		errList["Unmarshal_error"] = "Cannot unmarshal body"
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
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

	// check if the user exist:
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

	matchup.AuthorID = uid //the authenticated user is the one creating the matchup

	matchup.Prepare()
	errorMessages := matchup.Validate()
	if len(errorMessages) > 0 {
		errList = errorMessages
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
			"error":  errList,
		})
		return
	}

	// create the matchup
	tx := server.DB.Begin()
	matchupCreated, err := matchup.SaveMatchup(tx)
	if err != nil {
		//tx.Rollback()
		errList := formaterror.FormatError(err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  errList,
		})
		return
	}
	tx.Commit()

	// Retrieve the comments for the created matchup
	comment := models.Comment{}
	comments, err := comment.GetComments(server.DB, matchupCreated.ID)
	if err != nil {
		errList["No_comments"] = "No Comments Found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":   http.StatusCreated,
		"response": toMatchupResponse(matchupCreated, *comments),
	})
}

func (server *Server) GetMatchups(c *gin.Context) {
	matchup := models.Matchup{}

	matchups, err := matchup.FindAllMatchups(server.DB)
	if err != nil {
		errList["No_matchup"] = "No Matchup Found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	// Map the Matchup objects to MatchupResponse objects
	response := make([]MatchupResponse, len(matchups))
	for i, m := range matchups {
		// Retrieve the comments for each matchup
		comment := models.Comment{}
		comments, err := comment.GetComments(server.DB, m.ID)
		if err != nil {
			errList["No_comments"] = "No Comments Found"
			c.JSON(http.StatusNotFound, gin.H{
				"status": http.StatusNotFound,
				"error":  errList,
			})
			return
		}
		response[i] = *toMatchupResponse(&m, *comments)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": response,
	})
}

func (server *Server) GetMatchup(c *gin.Context) {
	matchupID := c.Param("id")
	mid, err := uuid.Parse(matchupID)
	if err != nil {
		errList["Invalid_request"] = "Invalid Request"
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  errList,
		})
		return
	}

	matchup := models.Matchup{}
	matchupReceived, err := matchup.FindMatchupByID(server.DB, mid)
	if err != nil {
		errList["No_matchup"] = "No Matchup Found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	// Retrieve the comments for the matchup
	comment := models.Comment{}
	comments, err := comment.GetComments(server.DB, mid)
	if err != nil {
		errList["No_comments"] = "No Comments Found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	// Add the comments to the matchup response
	matchupResponse := toMatchupResponse(matchupReceived, *comments)

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": matchupResponse,
	})
}

func (server *Server) UpdateMatchup(c *gin.Context) {

	//clear previous error if any
	errList = map[string]string{}

	matchupID := c.Param("id")
	// Check if the matchup id is valid
	mid, err := uuid.Parse(matchupID)

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
	//Check if the matchup exist
	origMatchup := models.Matchup{}
	err = server.DB.Model(models.Matchup{}).Where("id = ?", mid.String()).Take(&origMatchup).Error
	if err != nil {
		errList["No_matchup"] = "No matchup Found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}
	if uid != origMatchup.AuthorID {
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
	matchup := models.Matchup{}
	err = json.Unmarshal(body, &matchup)
	if err != nil {
		errList["Unmarshal_error"] = "Cannot unmarshal body"
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
			"error":  errList,
		})
		return
	}
	matchup.ID = origMatchup.ID //this is important to tell the model the matchup id to update, the other update field are set above
	matchup.AuthorID = origMatchup.AuthorID

	matchup.Prepare()
	errorMessages := matchup.Validate()
	if len(errorMessages) > 0 {
		errList = errorMessages
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
			"error":  errList,
		})
		return
	}
	matchupUpdated, err := matchup.UpdateMatchup(server.DB)
	if err != nil {
		errList := formaterror.FormatError(err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  errList,
		})
		return
	}

	// Retrieve the comments for the updated matchup
	comment := models.Comment{}
	comments, err := comment.GetComments(server.DB, matchupUpdated.ID)
	if err != nil {
		errList["No_comments"] = "No Comments Found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": toMatchupResponse(matchupUpdated, *comments), // Pass the comments slice here
	})
}

func (server *Server) DeleteMatchup(c *gin.Context) {

	matchupID := c.Param("id")
	// Is a valid matchup id given to us?
	mid, err := uuid.Parse(matchupID)
	if err != nil {
		errList["Invalid_request"] = "Invalid Request"
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  errList,
		})
		return
	}

	fmt.Println("this is delete matchup sir")

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
	// Check if the matchup exist
	matchup := models.Matchup{}
	err = server.DB.Model(models.Matchup{}).Where("id = ?", mid).Take(&matchup).Error
	if err != nil {
		errList["No_matchup"] = "No Matchup Found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}
	// Is the authenticated user, the owner of this matchup?
	if uid != matchup.AuthorID {
		errList["Unauthorized"] = "Unauthorized"
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": http.StatusUnauthorized,
			"error":  errList,
		})
		return
	}
	// If all the conditions are met, delete the matchup
	_, err = matchup.DeleteMatchup(server.DB)
	if err != nil {
		errList["Other_error"] = "Please try again later"
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  errList,
		})
		return
	}
	comment := models.Comment{}
	like := models.Like{}

	// Also delete the likes and the comments that this matchup have:
	_, err = comment.DeleteMatchupComments(server.DB, mid)
	if err != nil {
		errList["Other_error"] = "Please try again later"
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  errList,
		})
		return
	}
	_, err = like.DeleteMatchupLikes(server.DB, mid)
	if err != nil {
		errList["Other_error"] = "Please try again later"
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  errList,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": "Matchup deleted",
	})
}

func (server *Server) GetUserMatchups(c *gin.Context) {
	userID := c.Param("id")
	uid, err := uuid.Parse(userID)
	if err != nil {
		errList["Invalid_request"] = "Invalid Request"
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  errList,
		})
		return
	}

	matchup := models.Matchup{}
	matchups, err := matchup.FindUserMatchups(server.DB, uid)
	if err != nil {
		errList["No_matchup"] = "No Matchup Found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	// Map the Matchup objects to MatchupResponse objects
	response := make([]MatchupResponse, len(*matchups))
	for i, m := range *matchups {
		// Retrieve the comments for each matchup
		comment := models.Comment{}
		comments, err := comment.GetComments(server.DB, m.ID)
		if err != nil {
			errList["No_comments"] = "No Comments Found"
			c.JSON(http.StatusNotFound, gin.H{
				"status": http.StatusNotFound,
				"error":  errList,
			})
			return
		}
		response[i] = *toMatchupResponse(&m, *comments)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": response,
	})
}

func (server *Server) IncrementMatchupItemVotes(c *gin.Context) {
	itemID := c.Param("id")

	// Convert itemID to uuid
	id, err := uuid.Parse(itemID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  "Invalid item ID",
		})
		return
	}

	// Fetch the existing item
	var item models.MatchupItem
	err = server.DB.Model(&models.MatchupItem{}).Where("id = ?", id).Take(&item).Error
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  "MatchupItem not found",
		})
		return
	}

	// Increment the votes field
	item.Votes++

	// Update the item in the database
	err = server.DB.Model(&models.MatchupItem{}).Where("id = ?", id).Updates(models.MatchupItem{Votes: item.Votes}).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  "Error updating votes",
		})
		return
	}

	// Return the updated item
	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": item,
	})
}

func (server *Server) DeleteMatchupItem(c *gin.Context) {
	itemID := c.Param("id")

	// Convert itemID to uuid
	id, err := uuid.Parse(itemID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  "Invalid item ID",
		})
		return
	}

	// Fetch the existing item
	var item models.MatchupItem
	err = server.DB.Model(&models.MatchupItem{}).Where("id = ?", id).Take(&item).Error
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  "MatchupItem not found",
		})
		return
	}

	// Delete the item from the database
	err = server.DB.Delete(&models.MatchupItem{}, "id = ?", id).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  "Error deleting item",
		})
		return
	}

	// Return a success message
	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": "MatchupItem deleted successfully",
	})
}

func (server *Server) AddItemToMatchup(c *gin.Context) {
	matchupID := c.Param("matchup_id")

	// Convert matchupID to uuid
	id, err := uuid.Parse(matchupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  "Invalid matchup ID",
		})
		return
	}

	// Check if the matchup exists
	var matchup models.Matchup
	err = server.DB.Model(&models.Matchup{}).Where("id = ?", id).Take(&matchup).Error
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  "Matchup not found",
		})
		return
	}

	// Read the new item from the request body
	var item models.MatchupItem
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
			"error":  "Invalid JSON",
		})
		return
	}

	// Set the foreign key relationship (matchup_id)
	item.MatchupID = id

	// Save the new item in the database
	err = server.DB.Create(&item).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  "Error saving the new item",
		})
		return
	}

	// Return the created item
	c.JSON(http.StatusCreated, gin.H{
		"status":   http.StatusCreated,
		"response": item,
	})
}
