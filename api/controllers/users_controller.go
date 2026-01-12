package controllers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"Matchup/models"
	"Matchup/security"
	"Matchup/utils/fileformat"
	"Matchup/utils/formaterror"
	httpctx "Matchup/utils/httpctx"

	aws2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type BucketBasics struct {
	S3Client *s3.Client
}

func userToResponse(user *models.User) map[string]interface{} {
	return map[string]interface{}{
		"id":          user.ID,
		"username":    user.Username,
		"email":       user.Email,
		"avatar_path": user.AvatarPath,
		"is_admin":    user.IsAdmin,
		"created_at":  user.CreatedAt,
		"updated_at":  user.UpdatedAt,
	}
}

func buildPagination(page, limit int, total int64) map[string]interface{} {
	totalPages := 0
	if limit > 0 {
		totalPages = int((total + int64(limit) - 1) / int64(limit))
	}

	return map[string]interface{}{
		"page":        page,
		"limit":       limit,
		"total":       total,
		"total_pages": totalPages,
	}
}

// CreateUser handles user registration
func (server *Server) CreateUser(c *gin.Context) {
	var user models.User

	// Use ShouldBindJSON to parse and validate JSON input
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user.IsAdmin = false
	if strings.TrimSpace(user.Username) == "" {
		parts := strings.SplitN(strings.TrimSpace(user.Email), "@", 2)
		if len(parts) > 0 && parts[0] != "" {
			user.Username = parts[0]
		}
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

	c.JSON(http.StatusCreated, gin.H{
		"status":   http.StatusCreated,
		"response": userToResponse(userCreated),
	})
}

// GetCurrentUser returns the authenticated user's profile
func (server *Server) GetCurrentUser(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	uid, ok := userID.(uint)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user context"})
		return
	}

	user := models.User{}
	userGotten, err := user.FindUserByID(server.DB, uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": userToResponse(userGotten),
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
		userCopy := user
		userResponses[i] = userToResponse(&userCopy)
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

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": userToResponse(userGotten),
	})
}

// UpdateAvatar allows a user to update their avatar image
func (server *Server) UpdateAvatar(c *gin.Context) {
	// 1) Parse and validate user ID
	userID := c.Param("id")
	uid, err := strconv.ParseUint(userID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}
	requestorID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	if requestorID != uint(uid) && !httpctx.IsAdminRequest(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// 2) Pull and validate the uploaded file
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

	// 3) Generate S3 key under UserProfilePics prefix
	filePath := fileformat.UniqueFormat(file.Filename)
	key := "UserProfilePics/" + filePath

	// 4) Get bucket & region from env
	rawBucket := os.Getenv("S3_BUCKET") // bucket name only, e.g. "my-matchup-bucket"
	bucketName := strings.SplitN(rawBucket, "/", 2)[0]
	if bucketName == "" {
		log.Printf("S3_BUCKET env var is empty or invalid: '%s'", rawBucket)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Server configuration error"})
		return
	}

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-2"
	}

	// 5) Build AWS config with explicit static credentials if provided
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	sessionToken := os.Getenv("AWS_SESSION_TOKEN") // optional

	var cfg aws2.Config
	if accessKey != "" && secretKey != "" {
		// Use static credentials from env (works inside Docker / local)
		credsProvider := credentials.NewStaticCredentialsProvider(accessKey, secretKey, sessionToken)
		cfg, err = config.LoadDefaultConfig(
			context.TODO(),
			config.WithRegion(region),
			config.WithCredentialsProvider(credsProvider),
		)
	} else {
		// Fall back to default chain (e.g., EC2 role) if no keys present
		cfg, err = config.LoadDefaultConfig(
			context.TODO(),
			config.WithRegion(region),
		)
	}

	if err != nil {
		log.Printf("AWS config load error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AWS configuration error"})
		return
	}
	log.Printf("DEBUG UPLOAD: AWS_REGION: '%s'", os.Getenv("AWS_REGION"))
	log.Printf("DEBUG UPLOAD: AWS_ACCESS_KEY_ID: '%s'", os.Getenv("AWS_ACCESS_KEY_ID"))
	log.Printf("DEBUG UPLOAD: AWS_SECRET_ACCESS_KEY length: %d", len(os.Getenv("AWS_SECRET_ACCESS_KEY")))
	log.Printf("DEBUG UPLOAD: S3_BUCKET: '%s'", os.Getenv("S3_BUCKET"))

	// 6) Instantiate S3 client (path-style optional; most AWS buckets are fine without)
	s3Client := s3.NewFromConfig(cfg)

	// 7) Upload to S3
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

	// 8) Save avatar path in DB (just the filename; URL is built on read)
	user := models.User{AvatarPath: filePath}
	updatedUser, err := user.UpdateAUserAvatar(server.DB, uint(uid))
	if err != nil {
		log.Printf("DB update failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cannot save image, please try again later"})
		return
	}

	// 9) Build a public S3 URL for the response
	fullURL := fmt.Sprintf(
		"https://%s.s3.%s.amazonaws.com/%s",
		bucketName,
		region,
		key,
	)
	updatedUser.AvatarPath = fullURL

	// 10) Build response
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "response": userToResponse(updatedUser)})
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
	requestorID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	if requestorID != uint(uid) && !httpctx.IsAdminRequest(c) {
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

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": userToResponse(updatedUser),
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
	requestorID, ok := httpctx.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	if requestorID != uint(uid) && !httpctx.IsAdminRequest(c) {
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

// AdminListUsers returns a paginated list of users for the admin console.
func (server *Server) AdminListUsers(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 || limit > 100 {
		limit = 20
	}

	offset := (page - 1) * limit
	search := strings.TrimSpace(c.Query("search"))

	query := server.DB.Model(&models.User{})
	if search != "" {
		like := fmt.Sprintf("%%%s%%", search)
		query = query.Where("username ILIKE ? OR email ILIKE ?", like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to count users"})
		return
	}

	var users []models.User
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to fetch users"})
		return
	}

	userResponses := make([]map[string]interface{}, len(users))
	for i := range users {
		userResponses[i] = userToResponse(&users[i])
	}

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"response": gin.H{
			"users":      userResponses,
			"pagination": buildPagination(page, limit, total),
		},
	})
}

// AdminUpdateUserRole allows an admin to set/unset the admin flag on a user.
func (server *Server) AdminUpdateUserRole(c *gin.Context) {
	userID := c.Param("id")
	uid, err := strconv.ParseUint(userID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var payload struct {
		IsAdmin *bool `json:"is_admin"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	if payload.IsAdmin == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "is_admin is required"})
		return
	}

	var user models.User
	if err := server.DB.First(&user, uint(uid)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to load user"})
		return
	}

	if err := server.DB.Model(&user).Update("is_admin", *payload.IsAdmin).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to update user role"})
		return
	}

	user.IsAdmin = *payload.IsAdmin

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"response": userToResponse(&user),
	})
}

// AdminDeleteUser deletes a user and their content.
func (server *Server) AdminDeleteUser(c *gin.Context) {
	userID := c.Param("id")
	uid, err := strconv.ParseUint(userID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}
	id := uint(uid)

	var user models.User
	if err := server.DB.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to load user"})
		return
	}

	var brackets []models.Bracket
	if err := server.DB.Where("author_id = ?", id).Find(&brackets).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to load user brackets"})
		return
	}
	for i := range brackets {
		if err := server.deleteBracketCascade(&brackets[i]); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user brackets"})
			return
		}
	}

	var matchups []models.Matchup
	if err := server.DB.Where("author_id = ? AND bracket_id IS NULL", id).Find(&matchups).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to load user matchups"})
		return
	}
	for i := range matchups {
		if err := server.deleteMatchupCascade(&matchups[i]); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user matchups"})
			return
		}
	}

	if err := server.DB.Where("user_id = ?", id).Delete(&models.MatchupVote{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user votes"})
		return
	}
	if err := server.DB.Where("user_id = ?", id).Delete(&models.Like{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user likes"})
		return
	}
	if err := server.DB.Where("user_id = ?", id).Delete(&models.BracketLike{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user bracket likes"})
		return
	}
	if err := server.DB.Where("user_id = ?", id).Delete(&models.BracketComment{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user bracket comments"})
		return
	}
	if err := server.DB.Where("user_id = ?", id).Delete(&models.Comment{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user comments"})
		return
	}

	if _, err := user.DeleteAUser(server.DB, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User deleted"})
}
