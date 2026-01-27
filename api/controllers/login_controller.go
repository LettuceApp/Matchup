package controllers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"Matchup/auth"
	"Matchup/models"
	"Matchup/security"
	"Matchup/utils/formaterror"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// Login godoc
// @Summary      Log in
// @Description  Authenticate with email and password
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        credentials  body      LoginRequest  true  "Login payload"
// @Success      200          {object}  LoginResponseEnvelope
// @Failure      422          {object}  ErrorResponse
// @Router       /login [post]
func (server *Server) Login(c *gin.Context) {

	//clear previous error if any
	errList = map[string]string{}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status":      http.StatusUnprocessableEntity,
			"first error": "Unable to get request",
		})
		return
	}
	user := models.User{}
	err = json.Unmarshal(body, &user)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
			"error":  "Cannot unmarshal body",
		})
		return
	}
	user.Prepare()
	errorMessages := user.Validate("login")
	if len(errorMessages) > 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
			"error":  errorMessages,
		})
		return
	}
userData, internalID, err := server.SignIn(user.Email, user.Password)
	if err != nil {
		formattedError := formaterror.FormatError(err.Error())
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": http.StatusUnprocessableEntity,
			"error":  formattedError,
		})
		return
	}

if deviceID, err := c.Cookie(anonymousDeviceCookieName); err == nil && deviceID != "" {
	if internalID != 0 {
		if err := mergeDeviceVotesToUser(server.DB, internalID, deviceID); err != nil {
			log.Printf("merge anonymous votes: %v", err)
		}
	}
}

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": userData,
	})
}

func (server *Server) SignIn(email, password string) (map[string]interface{}, uint, error) {

	var err error

	userData := make(map[string]interface{})

	user := models.User{}

	normalizedEmail := strings.ToLower(email)
	err = server.DB.Model(models.User{}).Where("lower(email) = ?", normalizedEmail).Take(&user).Error
	if err != nil {
		fmt.Println("this is the error getting the user: ", err)
		return nil, 0, err
	}
	err = security.VerifyPassword(user.Password, password)
	if err != nil && err == bcrypt.ErrMismatchedHashAndPassword {
		fmt.Println("this is the error hashing the password: ", err)
		return nil, 0, err
	}
	token, err := auth.CreateToken(user.ID)
	if err != nil {
		fmt.Println("this is the error creating the token: ", err)
		return nil, 0, err
	}
userData["token"] = token
userData["id"] = user.PublicID
userData["email"] = user.Email
	userData["avatar_path"] = user.AvatarPath
	userData["username"] = user.Username
	userData["is_admin"] = user.IsAdmin
userData["is_private"] = user.IsPrivate

return userData, user.ID, nil
}
