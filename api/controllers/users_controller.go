package controllers

import (
	"bytes"
	"net/http"
	"os"
	"strconv"
	"strings"

	"Matchup/api/models"
	"Matchup/api/security"
	"Matchup/api/utils/fileformat"
	"Matchup/api/utils/formaterror"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gin-gonic/gin"
)

// CreateUser handles user registration
func (server *Server) CreateUser(c *gin.Context) {
	var user models.User

	// Use ShouldBindJSON to parse and validate JSON input
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user.Prepare()
	errorMessages := user.Validate("")
	if len(errorMessages) > 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"errors": errorMessages})
		return
	}

	userCreated, err := user.SaveUser(server.DB)
	if err != nil {
		formattedError := formaterror.FormatError(err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"errors": formattedError})
		return
	}

	// Prepare response without the password
	userResponse := map[string]interface{}{
		"id":          userCreated.ID,
		"username":    userCreated.Username,
		"email":       userCreated.Email,
		"avatar_path": userCreated.AvatarPath,
		"created_at":  userCreated.CreatedAt,
		"updated_at":  userCreated.UpdatedAt,
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":   http.StatusCreated,
		"response": userResponse,
	})
}

// GetUsers retrieves all users
func (server *Server) GetUsers(c *gin.Context) {
	user := models.User{}

	users, err := user.FindAllUsers(server.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No users found"})
		return
	}

	// Prepare response without passwords
	userResponses := make([]map[string]interface{}, len(*users))
	for i, user := range *users {
		userResponses[i] = map[string]interface{}{
			"id":          user.ID,
			"username":    user.Username,
			"email":       user.Email,
			"avatar_path": user.AvatarPath,
			"created_at":  user.CreatedAt,
			"updated_at":  user.UpdatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": userResponses,
	})
}

// GetUser retrieves a user by ID
func (server *Server) GetUser(c *gin.Context) {
	userID := c.Param("id")

	uid, err := strconv.ParseUint(userID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	user := models.User{}

	userGotten, err := user.FindUserByID(server.DB, uint(uid))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Prepare response without the password
	userResponse := map[string]interface{}{
		"id":          userGotten.ID,
		"username":    userGotten.Username,
		"email":       userGotten.Email,
		"avatar_path": userGotten.AvatarPath,
		"created_at":  userGotten.CreatedAt,
		"updated_at":  userGotten.UpdatedAt,
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": userResponse,
	})
}

// UpdateAvatar allows a user to update their avatar image
func (server *Server) UpdateAvatar(c *gin.Context) {
	userID := c.Param("id")

	uid, err := strconv.ParseUint(userID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Get user ID from context (set by authentication middleware)
	tokenID, exists := c.Get("userID")
	if !exists || tokenID != uint(uid) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file"})
		return
	}

	f, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot open file"})
		return
	}
	defer f.Close()

	size := file.Size
	maxSize := int64(512000) // 500KB
	if size > maxSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File too large. Please upload a file less than 500KB"})
		return
	}

	buffer := make([]byte, size)
	f.Read(buffer)
	fileBytes := bytes.NewReader(buffer)
	fileType := http.DetectContentType(buffer)
	if !strings.HasPrefix(fileType, "image") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Please upload a valid image"})
		return
	}

	filePath := fileformat.UniqueFormat(file.Filename)
	path := "/profile-photos/" + filePath

	// Initialize AWS S3 client
	s3Config := &aws.Config{
		Credentials: credentials.NewStaticCredentials(
			os.Getenv("DO_SPACES_KEY"), os.Getenv("DO_SPACES_SECRET"), os.Getenv("DO_SPACES_TOKEN")),
		Endpoint: aws.String(os.Getenv("DO_SPACES_ENDPOINT")),
		Region:   aws.String(os.Getenv("DO_SPACES_REGION")),
	}
	newSession, err := session.NewSession(s3Config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create AWS session"})
		return
	}
	s3Client := s3.New(newSession)

	// Upload file to S3
	params := &s3.PutObjectInput{
		Bucket:        aws.String("chodapi"),
		Key:           aws.String(path),
		Body:          fileBytes,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(fileType),
		ACL:           aws.String("public-read"),
	}

	_, err = s3Client.PutObject(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload image"})
		return
	}

	// Update user's avatar path
	user := models.User{}
	user.AvatarPath = filePath
	updatedUser, err := user.UpdateAUserAvatar(server.DB, uint(uid))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cannot save image, please try again later"})
		return
	}

	// Prepare response without the password
	userResponse := map[string]interface{}{
		"id":          updatedUser.ID,
		"username":    updatedUser.Username,
		"email":       updatedUser.Email,
		"avatar_path": updatedUser.AvatarPath,
		"created_at":  updatedUser.CreatedAt,
		"updated_at":  updatedUser.UpdatedAt,
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": userResponse,
	})
}

// UpdateUser allows a user to update their email and password
func (server *Server) UpdateUser(c *gin.Context) {
	userID := c.Param("id")

	uid, err := strconv.ParseUint(userID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Get user ID from context (set by authentication middleware)
	tokenID, exists := c.Get("userID")
	if !exists || tokenID != uint(uid) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var requestBody map[string]string
	if err := c.ShouldBindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot parse request body"})
		return
	}

	// Retrieve existing user data
	formerUser := models.User{}
	err = server.DB.Model(&models.User{}).Where("id = ?", uint(uid)).Take(&formerUser).Error
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	newUser := models.User{}
	newUser.Username = formerUser.Username // Usernames are immutable

	// Handle password change if requested
	if currentPassword, ok := requestBody["current_password"]; ok {
		if newPassword, ok := requestBody["new_password"]; ok {
			if len(newPassword) < 6 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Password should be at least 6 characters"})
				return
			}
			err = security.VerifyPassword(formerUser.Password, currentPassword)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Current password is incorrect"})
				return
			}
			newUser.Password = newPassword
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "New password is required"})
			return
		}
	}

	// Update email if provided
	if email, ok := requestBody["email"]; ok {
		newUser.Email = email
	} else {
		newUser.Email = formerUser.Email
	}

	newUser.Prepare()
	errorMessages := newUser.Validate("update")
	if len(errorMessages) > 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"errors": errorMessages})
		return
	}

	updatedUser, err := newUser.UpdateAUser(server.DB, uint(uid))
	if err != nil {
		formattedError := formaterror.FormatError(err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"errors": formattedError})
		return
	}

	// Prepare response without the password
	userResponse := map[string]interface{}{
		"id":          updatedUser.ID,
		"username":    updatedUser.Username,
		"email":       updatedUser.Email,
		"avatar_path": updatedUser.AvatarPath,
		"created_at":  updatedUser.CreatedAt,
		"updated_at":  updatedUser.UpdatedAt,
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": userResponse,
	})
}

// DeleteUser deletes a user and their associated data
func (server *Server) DeleteUser(c *gin.Context) {
	userID := c.Param("id")

	uid, err := strconv.ParseUint(userID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Get user ID from context (set by authentication middleware)
	tokenID, exists := c.Get("userID")
	if !exists || tokenID != uint(uid) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	user := models.User{}
	_, err = user.DeleteAUser(server.DB, uint(uid))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Please try again later"})
		return
	}

	// Return a success message
	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": "User deleted",
	})
}
