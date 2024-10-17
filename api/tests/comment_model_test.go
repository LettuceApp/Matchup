package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"Matchup/api/controllers"
	"Matchup/api/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestCreateComment tests the creation of a comment
func TestCreateComment(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a new router and server instance
	r := gin.Default()
	server := &controllers.Server{}

	// Setup SQLite as an in-memory database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to in-memory database: %v", err)
	}
	server.DB = db
	server.DB.AutoMigrate(&models.User{}, &models.Matchup{}, &models.Like{}, &models.MatchupItem{}, &models.Comment{})

	// Apply routes directly to the main router
	r.POST("/api/v1/users", server.CreateUser)
	r.POST("/api/v1/login", server.Login)
	r.POST("/api/v1/matchups", server.CreateMatchup)

	// Setup authenticated router for comment routes
	authenticatedRouter := r.Group("/api/v1")
	authenticatedRouter.Use(AuthMiddlewareForTests(1))
	authenticatedRouter.POST("/matchups/:matchup_id/comments", server.CreateComment)

	// Create and authenticate user
	mockUser := map[string]string{
		"username": "testuser",
		"email":    "testuser@example.com",
		"password": "password123",
	}
	requestBody, _ := json.Marshal(mockUser)
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Parse the response to get user ID
	var responseBody map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &responseBody)
	userID := uint(responseBody["response"].(map[string]interface{})["id"].(float64))

	// Login to get the token
	loginPayload := map[string]string{
		"email":    mockUser["email"],
		"password": mockUser["password"],
	}
	loginRequestBody, _ := json.Marshal(loginPayload)
	loginReq, _ := http.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewBuffer(loginRequestBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	r.ServeHTTP(loginW, loginReq)

	var loginResponseBody map[string]interface{}
	_ = json.Unmarshal(loginW.Body.Bytes(), &loginResponseBody)
	token, exists := loginResponseBody["response"].(map[string]interface{})["token"].(string)
	if !exists {
		t.Fatalf("Token not found in login response")
	}

	// Create a matchup using the authenticated token
	mockMatchup := map[string]interface{}{
		"title":     "Test Matchup",
		"content":   "Some content",
		"author_id": userID,
	}
	matchupRequestBody, _ := json.Marshal(mockMatchup)
	matchupReq, _ := http.NewRequest(http.MethodPost, "/api/v1/matchups", bytes.NewBuffer(matchupRequestBody))
	matchupReq.Header.Set("Content-Type", "application/json")
	matchupReq.Header.Set("Authorization", "Bearer "+token)
	matchupW := httptest.NewRecorder()
	r.ServeHTTP(matchupW, matchupReq)

	// Parse the matchup response to get the matchup ID
	var matchupResponse map[string]interface{}
	_ = json.Unmarshal(matchupW.Body.Bytes(), &matchupResponse)
	matchupData := matchupResponse["response"].(map[string]interface{})
	matchupID := int(matchupData["id"].(float64))

	// Add a comment
	mockComment := map[string]interface{}{
		"body": "This is a test comment",
	}
	commentRequestBody, _ := json.Marshal(mockComment)
	commentReq, _ := http.NewRequest(http.MethodPost, "/api/v1/matchups/"+strconv.Itoa(matchupID)+"/comments", bytes.NewBuffer(commentRequestBody))
	commentReq.Header.Set("Content-Type", "application/json")
	commentReq.Header.Set("Authorization", "Bearer "+token)
	commentW := httptest.NewRecorder()
	r.ServeHTTP(commentW, commentReq) // Use the main router 'r' instead of 'authenticatedRouter'

	// Check response status for comment creation
	assert.Equal(t, http.StatusCreated, commentW.Code)
}

func TestGetComments(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a new router and server instance
	r := gin.Default()
	server := &controllers.Server{}

	// Setup SQLite as an in-memory database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to in-memory database: %v", err)
	}
	server.DB = db
	server.DB.AutoMigrate(&models.User{}, &models.Matchup{}, &models.Like{}, &models.MatchupItem{}, &models.Comment{})

	// Apply routes directly to the main router
	r.POST("/api/v1/users", server.CreateUser)
	r.POST("/api/v1/login", server.Login)
	r.POST("/api/v1/matchups", server.CreateMatchup)

	// Setup authenticated router for comment routes
	authenticatedRouter := r.Group("/api/v1")
	authenticatedRouter.Use(AuthMiddlewareForTests(1))
	authenticatedRouter.POST("/matchups/:matchup_id/comments", server.CreateComment)
	authenticatedRouter.GET("/matchups/:id/comments", server.GetComments)

	// Create and authenticate user
	mockUser := map[string]string{
		"username": "testuser",
		"email":    "testuser@example.com",
		"password": "password123",
	}
	requestBody, _ := json.Marshal(mockUser)
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Parse the response to get user ID
	var responseBody map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &responseBody)
	userID := uint(responseBody["response"].(map[string]interface{})["id"].(float64))

	// Login to get the token
	loginPayload := map[string]string{
		"email":    mockUser["email"],
		"password": mockUser["password"],
	}
	loginRequestBody, _ := json.Marshal(loginPayload)
	loginReq, _ := http.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewBuffer(loginRequestBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	r.ServeHTTP(loginW, loginReq)

	var loginResponseBody map[string]interface{}
	_ = json.Unmarshal(loginW.Body.Bytes(), &loginResponseBody)
	token, exists := loginResponseBody["response"].(map[string]interface{})["token"].(string)
	if !exists {
		t.Fatalf("Token not found in login response")
	}

	// Create a matchup using the authenticated token
	mockMatchup := map[string]interface{}{
		"title":     "Test Matchup",
		"content":   "Some content",
		"author_id": userID,
	}
	matchupRequestBody, _ := json.Marshal(mockMatchup)
	matchupReq, _ := http.NewRequest(http.MethodPost, "/api/v1/matchups", bytes.NewBuffer(matchupRequestBody))
	matchupReq.Header.Set("Content-Type", "application/json")
	matchupReq.Header.Set("Authorization", "Bearer "+token)
	matchupW := httptest.NewRecorder()
	r.ServeHTTP(matchupW, matchupReq)

	// Parse the matchup response to get the matchup ID
	var matchupResponse map[string]interface{}
	_ = json.Unmarshal(matchupW.Body.Bytes(), &matchupResponse)
	matchupData := matchupResponse["response"].(map[string]interface{})
	matchupID := int(matchupData["id"].(float64))

	// Add a comment
	mockComment := map[string]interface{}{
		"body": "This is a test comment",
	}
	commentRequestBody, _ := json.Marshal(mockComment)
	commentReq, _ := http.NewRequest(http.MethodPost, "/api/v1/matchups/"+strconv.Itoa(matchupID)+"/comments", bytes.NewBuffer(commentRequestBody))
	commentReq.Header.Set("Content-Type", "application/json")
	commentReq.Header.Set("Authorization", "Bearer "+token)
	commentW := httptest.NewRecorder()
	r.ServeHTTP(commentW, commentReq)

	// Check if the comment was created successfully
	assert.Equal(t, http.StatusCreated, commentW.Code)

	// Get comments for the matchup using the main router
	getCommentsReq, _ := http.NewRequest(http.MethodGet, "/api/v1/matchups/"+strconv.Itoa(matchupID)+"/comments", nil)
	getCommentsReq.Header.Set("Authorization", "Bearer "+token)
	getCommentsW := httptest.NewRecorder()
	r.ServeHTTP(getCommentsW, getCommentsReq)

	// Check response status for retrieving comments
	assert.Equal(t, http.StatusOK, getCommentsW.Code)

	// Parse the response and validate the content
	var commentsResponse map[string]interface{}
	err = json.Unmarshal(getCommentsW.Body.Bytes(), &commentsResponse)
	if err != nil {
		t.Fatalf("Error unmarshalling comments response: %v", err)
	}

	commentsData, exists := commentsResponse["response"].([]interface{})
	if !exists {
		t.Fatalf("Comments response does not contain 'response' field or is not an array")
	}

	// Assert there is at least one comment and validate its content
	assert.GreaterOrEqual(t, len(commentsData), 1)
	firstComment := commentsData[0].(map[string]interface{})
	assert.Equal(t, "This is a test comment", firstComment["body"])
}

func TestUpdateComment(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a new router and server instance
	r := gin.Default()
	server := &controllers.Server{}

	// Setup SQLite as an in-memory database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to in-memory database: %v", err)
	}
	server.DB = db
	server.DB.AutoMigrate(&models.User{}, &models.Matchup{}, &models.Like{}, &models.MatchupItem{}, &models.Comment{})

	// Apply routes directly to the main router
	r.POST("/api/v1/users", server.CreateUser)
	r.POST("/api/v1/login", server.Login)
	r.POST("/api/v1/matchups", server.CreateMatchup)

	// Setup authenticated router for comment routes
	authenticatedRouter := r.Group("/api/v1")
	authenticatedRouter.Use(AuthMiddlewareForTests(1))
	authenticatedRouter.POST("/matchups/:matchup_id/comments", server.CreateComment)
	authenticatedRouter.PUT("/comments/:id", server.UpdateComment)

	// Create and authenticate user
	mockUser := map[string]string{
		"username": "testuser",
		"email":    "testuser@example.com",
		"password": "password123",
	}
	requestBody, _ := json.Marshal(mockUser)
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Parse the response to get user ID
	var responseBody map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &responseBody)
	userID := uint(responseBody["response"].(map[string]interface{})["id"].(float64))

	// Login to get the token
	loginPayload := map[string]string{
		"email":    mockUser["email"],
		"password": mockUser["password"],
	}
	loginRequestBody, _ := json.Marshal(loginPayload)
	loginReq, _ := http.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewBuffer(loginRequestBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	r.ServeHTTP(loginW, loginReq)

	var loginResponseBody map[string]interface{}
	_ = json.Unmarshal(loginW.Body.Bytes(), &loginResponseBody)
	token, exists := loginResponseBody["response"].(map[string]interface{})["token"].(string)
	if !exists {
		t.Fatalf("Token not found in login response")
	}

	// Create a matchup using the authenticated token
	mockMatchup := map[string]interface{}{
		"title":     "Test Matchup",
		"content":   "Some content",
		"author_id": userID,
	}
	matchupRequestBody, _ := json.Marshal(mockMatchup)
	matchupReq, _ := http.NewRequest(http.MethodPost, "/api/v1/matchups", bytes.NewBuffer(matchupRequestBody))
	matchupReq.Header.Set("Content-Type", "application/json")
	matchupReq.Header.Set("Authorization", "Bearer "+token)
	matchupW := httptest.NewRecorder()
	r.ServeHTTP(matchupW, matchupReq)

	// Parse the matchup response to get the matchup ID
	var matchupResponse map[string]interface{}
	_ = json.Unmarshal(matchupW.Body.Bytes(), &matchupResponse)
	matchupData := matchupResponse["response"].(map[string]interface{})
	matchupID := int(matchupData["id"].(float64))

	// Add a comment
	mockComment := map[string]interface{}{
		"body": "This is a test comment",
	}
	commentRequestBody, _ := json.Marshal(mockComment)
	commentReq, _ := http.NewRequest(http.MethodPost, "/api/v1/matchups/"+strconv.Itoa(matchupID)+"/comments", bytes.NewBuffer(commentRequestBody))
	commentReq.Header.Set("Content-Type", "application/json")
	commentReq.Header.Set("Authorization", "Bearer "+token)
	commentW := httptest.NewRecorder()
	r.ServeHTTP(commentW, commentReq)

	// Check if the comment was created successfully
	assert.Equal(t, http.StatusCreated, commentW.Code)

	// Parse the comment response to get the comment ID
	var commentResponse map[string]interface{}
	_ = json.Unmarshal(commentW.Body.Bytes(), &commentResponse)
	commentData := commentResponse["response"].(map[string]interface{})
	commentID := int(commentData["id"].(float64))

	// Update the comment
	updatedComment := map[string]interface{}{
		"body":       "This is an updated comment",
		"matchup_id": matchupID,
		"user_id":    userID,
	}
	updateCommentRequestBody, _ := json.Marshal(updatedComment)
	updateCommentReq, _ := http.NewRequest(http.MethodPut, "/api/v1/comments/"+strconv.Itoa(commentID), bytes.NewBuffer(updateCommentRequestBody))
	updateCommentReq.Header.Set("Content-Type", "application/json")
	updateCommentReq.Header.Set("Authorization", "Bearer "+token)
	updateCommentW := httptest.NewRecorder()
	r.ServeHTTP(updateCommentW, updateCommentReq)

	// Check if the comment update was successful
	assert.Equal(t, http.StatusOK, updateCommentW.Code)

	// Parse the update response to verify the update
	var updateResponse map[string]interface{}
	err = json.Unmarshal(updateCommentW.Body.Bytes(), &updateResponse)
	if err != nil {
		t.Fatalf("Error unmarshalling update response: %v", err)
	}

	updatedCommentData := updateResponse["response"].(map[string]interface{})
	assert.Equal(t, "This is an updated comment", updatedCommentData["body"])
}

func TestDeleteComment(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a new router and server instance
	r := gin.Default()
	server := &controllers.Server{}

	// Setup SQLite as an in-memory database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to in-memory database: %v", err)
	}
	server.DB = db
	server.DB.AutoMigrate(&models.User{}, &models.Matchup{}, &models.Like{}, &models.MatchupItem{}, &models.Comment{})

	// Apply authentication middleware for comment routes
	authenticatedRouter := r.Group("/api/v1")
	authenticatedRouter.Use(AuthMiddlewareForTests(1))
	authenticatedRouter.POST("/matchups/:matchup_id/comments", server.CreateComment)
	authenticatedRouter.GET("/matchups/:id/comments", server.GetComments) // Update the route parameter here
	authenticatedRouter.DELETE("/comments/:id", server.DeleteComment)

	// Define other routes that don't need authentication
	r.POST("/api/v1/users", server.CreateUser)
	r.POST("/api/v1/login", server.Login)
	r.POST("/api/v1/matchups", server.CreateMatchup)

	// Create and authenticate user
	mockUser := map[string]string{
		"username": "testuser",
		"email":    "testuser@example.com",
		"password": "password123",
	}
	requestBody, _ := json.Marshal(mockUser)
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Parse the response to get user ID
	var responseBody map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &responseBody)
	userID := uint(responseBody["response"].(map[string]interface{})["id"].(float64))

	// Login to get the token
	loginPayload := map[string]string{
		"email":    mockUser["email"],
		"password": mockUser["password"],
	}
	loginRequestBody, _ := json.Marshal(loginPayload)
	loginReq, _ := http.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewBuffer(loginRequestBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	r.ServeHTTP(loginW, loginReq)

	var loginResponseBody map[string]interface{}
	_ = json.Unmarshal(loginW.Body.Bytes(), &loginResponseBody)
	token, exists := loginResponseBody["response"].(map[string]interface{})["token"].(string)
	if !exists {
		t.Fatalf("Token not found in login response")
	}

	// Create a matchup using the authenticated token
	mockMatchup := map[string]interface{}{
		"title":     "Test Matchup",
		"content":   "Some content",
		"author_id": userID,
	}
	matchupRequestBody, _ := json.Marshal(mockMatchup)
	matchupReq, _ := http.NewRequest(http.MethodPost, "/api/v1/matchups", bytes.NewBuffer(matchupRequestBody))
	matchupReq.Header.Set("Content-Type", "application/json")
	matchupReq.Header.Set("Authorization", "Bearer "+token)
	matchupW := httptest.NewRecorder()
	r.ServeHTTP(matchupW, matchupReq)

	// Parse the matchup response to get the matchup ID
	var matchupResponse map[string]interface{}
	_ = json.Unmarshal(matchupW.Body.Bytes(), &matchupResponse)
	matchupData := matchupResponse["response"].(map[string]interface{})
	matchupID := int(matchupData["id"].(float64))

	// Add a comment
	mockComment := map[string]interface{}{
		"body": "This is a test comment",
	}
	commentRequestBody, _ := json.Marshal(mockComment)
	commentReq, _ := http.NewRequest(http.MethodPost, "/api/v1/matchups/"+strconv.Itoa(matchupID)+"/comments", bytes.NewBuffer(commentRequestBody))
	commentReq.Header.Set("Content-Type", "application/json")
	commentReq.Header.Set("Authorization", "Bearer "+token)
	commentW := httptest.NewRecorder()
	r.ServeHTTP(commentW, commentReq)

	// Check if the comment was created successfully
	assert.Equal(t, http.StatusCreated, commentW.Code)

	// Parse the comment response to get the comment ID
	var commentResponse map[string]interface{}
	_ = json.Unmarshal(commentW.Body.Bytes(), &commentResponse)
	commentData := commentResponse["response"].(map[string]interface{})
	commentID := int(commentData["id"].(float64))

	// Delete the comment
	deleteCommentReq, _ := http.NewRequest(http.MethodDelete, "/api/v1/comments/"+strconv.Itoa(commentID), nil)
	deleteCommentReq.Header.Set("Authorization", "Bearer "+token)
	deleteCommentW := httptest.NewRecorder()
	r.ServeHTTP(deleteCommentW, deleteCommentReq)

	// Check if the comment was deleted successfully
	assert.Equal(t, http.StatusOK, deleteCommentW.Code)

	// Call GetComments to verify the comment was removed
	getCommentsReq, _ := http.NewRequest(http.MethodGet, "/api/v1/matchups/"+strconv.Itoa(matchupID)+"/comments", nil)
	getCommentsReq.Header.Set("Authorization", "Bearer "+token)
	getCommentsW := httptest.NewRecorder()
	r.ServeHTTP(getCommentsW, getCommentsReq)

	// Check if the comments were retrieved successfully
	assert.Equal(t, http.StatusOK, getCommentsW.Code)

	var getCommentsResponse map[string]interface{}
	err = json.Unmarshal(getCommentsW.Body.Bytes(), &getCommentsResponse)
	if err != nil {
		t.Fatalf("Error unmarshalling get comments response: %v", err)
	}

	comments, exists := getCommentsResponse["response"].([]interface{})
	if !exists || len(comments) != 0 {
		t.Fatalf("Expected no comments, found: %v", comments)
	}
}
