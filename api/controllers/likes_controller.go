package controllers

import (
	"fmt"
	"net/http"
	"strconv"

	"Matchup/api/auth"
	"Matchup/api/models"
	"Matchup/api/utils/formaterror"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (server *Server) LikeMatchup(c *gin.Context) {

	//clear previous error if any
	errList = map[string]string{}

	matchupID := c.Param("id")
	mid, err := strconv.ParseUint(matchupID, 10, 64)
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
	// check if the matchup exist:
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

	like := models.Like{}
	like.UserID = user.ID
	like.MatchupID = matchup.ID

	likeCreated, err := like.SaveLike(server.DB)
	if err != nil {
		formattedError := formaterror.FormatError(err.Error())
		errList = formattedError
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  errList,
		})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"status":   http.StatusCreated,
		"response": likeCreated,
	})
}

func (server *Server) GetLikes(c *gin.Context) {

	//clear previous error if any
	errList = map[string]string{}

	matchupID := c.Param("id")

	// Is a valid matchup id given to us?
	mid, err := uuid.Parse(matchupID)
	if err != nil {
		fmt.Println("this is the error: ", err)
		errList["Invalid_request"] = "Invalid Request"
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  errList,
		})
		return
	}

	// Check if the matchup exist:
	matchup := models.Matchup{}
	err = server.DB.Model(models.Matchup{}).Where("id = ?", mid).Take(&matchup).Error
	if err != nil {
		errList["No_mactchup"] = "No Matchup Found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}

	like := models.Like{}

	likes, err := like.GetLikesInfo(server.DB, mid)
	if err != nil {
		errList["No_likes"] = "No Likes found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": likes,
	})
}

func (server *Server) UnLikeMatchup(c *gin.Context) {

	likeID := c.Param("id")
	// Is a valid like id given to us?
	lid, err := strconv.ParseUint(likeID, 10, 64)
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
	// Check if the matchup exist
	like := models.Like{}
	err = server.DB.Model(models.Like{}).Where("id = ?", lid).Take(&like).Error
	if err != nil {
		errList["No_like"] = "No Like Found"
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  errList,
		})
		return
	}
	// Is the authenticated user, the owner of this matchup?
	if uid != like.UserID {
		errList["Unauthorized"] = "Unauthorized"
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": http.StatusUnauthorized,
			"error":  errList,
		})
		return
	}

	// If all the conditions are met, delete the matchup
	_, err = like.DeleteLike(server.DB)
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
		"response": "Like deleted",
	})
}
