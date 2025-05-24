package controllers

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"Matchup/api/models"
	"Matchup/api/security"
	"Matchup/api/utils/fileformat"
	"Matchup/api/utils/formaterror"

	aws2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
)

type BucketBasics struct {
	S3Client *s3.Client
}

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
	// Parse and validate user ID
	userID := c.Param("id")
	uid, err := strconv.ParseUint(userID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}
	tokenID, exists := c.Get("userID")
	if !exists || tokenID != uint(uid) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Pull and validate the uploaded file
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
	if size > 512_000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File too large (<500KB)"})
		return
	}

	buf := make([]byte, size)
	if _, err := f.Read(buf); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not read file"})
		return
	}
	fileType := http.DetectContentType(buf)
	if !strings.HasPrefix(fileType, "image/") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not an image"})
		return
	}

	// Generate S3 key under UserProfilePics prefix
	filePath := fileformat.UniqueFormat(file.Filename)
	key := "UserProfilePics/" + filePath

	// Determine bucket name, stripping any accidental path suffix
	rawBucket := os.Getenv("S3_BUCKET")
	bucketName := strings.SplitN(rawBucket, "/", 2)[0]
	if bucketName == "" {
		log.Printf("S3_BUCKET env var is empty or invalid: '%s'", rawBucket)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Server configuration error"})
		return
	}

	// Load AWS config (uses default credential chain + region)
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-2"
	}
	cfg, err := config.LoadDefaultConfig(
		context.TODO(),
		config.WithRegion(region),
	)
	if err != nil {
		log.Printf("AWS config load error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AWS configuration error"})
		return
	}

	// Instantiate S3 client using path-style addressing
	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	// Upload to S3
	_, err = s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:        aws2.String(bucketName),
		Key:           aws2.String(key),
		Body:          bytes.NewReader(buf),
		ContentLength: aws2.Int64(size),
		ContentType:   aws2.String(fileType),
	})
	if err != nil {
		log.Printf("S3 upload failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload image"})
		return
	}

	// Save avatar path in DB
	user := models.User{AvatarPath: filePath}
	updatedUser, err := user.UpdateAUserAvatar(server.DB, uint(uid))
	if err != nil {
		log.Printf("DB update failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cannot save image, please try again later"})
		return
	}
	// 7) Build a public S3 URL (virtual-host style)
	fullURL := fmt.Sprintf(
		"https://%s.s3.%s.amazonaws.com/%s",
		bucketName,
		region,
		key,
	)
	updatedUser.AvatarPath = fullURL
	log.Printf(updatedUser.AvatarPath)

	// Build response
	userResponse := map[string]interface{}{
		"id":          updatedUser.ID,
		"username":    updatedUser.Username,
		"email":       updatedUser.Email,
		"avatar_path": updatedUser.AvatarPath,
		"created_at":  updatedUser.CreatedAt,
		"updated_at":  updatedUser.UpdatedAt,
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "response": userResponse})
	log.Printf("Avatar updated successfully for user %d", updatedUser.ID)
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
