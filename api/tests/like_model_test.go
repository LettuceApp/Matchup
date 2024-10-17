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

func TestLikeMatchup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a new router and server instance
	r := gin.Default()
	server := &controllers.Server{}

	// Setup in-memory database
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
	authenticatedRouter := r.Group("/api/v1")
	authenticatedRouter.Use(AuthMiddlewareForTests(1))
	authenticatedRouter.POST("/likes/:id", server.LikeMatchup)

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

	// Add a like
	likeReq, _ := http.NewRequest(http.MethodPost, "/api/v1/likes/"+strconv.Itoa(matchupID), nil)
	likeReq.Header.Set("Authorization", "Bearer "+token)
	likeW := httptest.NewRecorder()
	r.ServeHTTP(likeW, likeReq)

	// Check response status for liking a matchup
	assert.Equal(t, http.StatusCreated, likeW.Code)
}

func TestUnlikeMatchup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a new router and server instance
	r := gin.Default()
	server := &controllers.Server{}

	// Use SQLite as an in-memory database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to in-memory database: %v", err)
	}
	server.DB = db

	// Auto-migrate the necessary models
	server.DB.AutoMigrate(&models.User{}, &models.Matchup{}, &models.Like{}, &models.MatchupItem{}, &models.Comment{})

	// Define routes
	r.POST("/api/v1/users", server.CreateUser)
	r.POST("/api/v1/login", server.Login)
	r.POST("/api/v1/matchups", server.CreateMatchup)
	r.POST("/likes/:id", server.LikeMatchup)
	r.DELETE("/likes/:id", server.UnLikeMatchup)

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

	// Like the matchup
	likeReq, _ := http.NewRequest(http.MethodPost, "/likes/"+strconv.Itoa(matchupID), nil)
	likeReq.Header.Set("Authorization", "Bearer "+token)
	likeW := httptest.NewRecorder()
	r.ServeHTTP(likeW, likeReq)

	// Check if the like was successful (expecting 201)
	assert.Equal(t, http.StatusCreated, likeW.Code)

	// Unlike the matchup
	unlikeReq, _ := http.NewRequest(http.MethodDelete, "/likes/"+strconv.Itoa(matchupID), nil)
	unlikeReq.Header.Set("Authorization", "Bearer "+token)
	unlikeW := httptest.NewRecorder()
	r.ServeHTTP(unlikeW, unlikeReq)

	// Check if the unlike was successful
	assert.Equal(t, http.StatusOK, unlikeW.Code)

	// Parse the unlike response to validate the message
	var unlikeResponse map[string]interface{}
	_ = json.Unmarshal(unlikeW.Body.Bytes(), &unlikeResponse)
	assert.Equal(t, "Like removed", unlikeResponse["response"])
}

func TestGetLikes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a new router and server instance
	r := gin.Default()
	server := &controllers.Server{}

	// Use SQLite as an in-memory database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to in-memory database: %v", err)
	}
	server.DB = db

	// Auto-migrate the necessary models
	server.DB.AutoMigrate(&models.User{}, &models.Matchup{}, &models.Like{}, &models.MatchupItem{}, &models.Comment{})

	// Define routes
	r.POST("/api/v1/users", server.CreateUser)
	r.POST("/api/v1/login", server.Login)
	r.POST("/api/v1/matchups", server.CreateMatchup)
	r.POST("/likes/:id", server.LikeMatchup)
	r.GET("/likes/matchups/:id", server.GetLikes)

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

	// Like the matchup
	likeReq, _ := http.NewRequest(http.MethodPost, "/likes/"+strconv.Itoa(matchupID), nil)
	likeReq.Header.Set("Authorization", "Bearer "+token)
	likeW := httptest.NewRecorder()
	r.ServeHTTP(likeW, likeReq)

	// Check if the like was successful
	assert.Equal(t, http.StatusCreated, likeW.Code)

	// Get likes for the matchup
	getLikesReq, _ := http.NewRequest(http.MethodGet, "/likes/matchups/"+strconv.Itoa(matchupID), nil)
	getLikesReq.Header.Set("Authorization", "Bearer "+token)
	getLikesW := httptest.NewRecorder()
	r.ServeHTTP(getLikesW, getLikesReq)

	// Check if the likes were retrieved successfully
	assert.Equal(t, http.StatusOK, getLikesW.Code)

	// Parse the response to validate likes
	var getLikesResponse map[string]interface{}
	_ = json.Unmarshal(getLikesW.Body.Bytes(), &getLikesResponse)
	likesData, exists := getLikesResponse["response"].([]interface{})
	if !exists || len(likesData) == 0 {
		t.Fatalf("Likes response does not contain 'response' field or is not an array")
	}

	// Validate the first like in the response
	firstLike := likesData[0].(map[string]interface{})
	assert.Equal(t, float64(userID), firstLike["user_id"])
	assert.Equal(t, float64(matchupID), firstLike["matchup_id"])
}

func TestGetMatchupLikes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a new router and server instance
	r := gin.Default()
	server := &controllers.Server{}

	// Use SQLite as an in-memory database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to in-memory database: %v", err)
	}
	server.DB = db

	// Auto-migrate the necessary models
	server.DB.AutoMigrate(&models.User{}, &models.Matchup{}, &models.Like{}, &models.MatchupItem{}, &models.Comment{})

	// Define routes
	r.POST("/api/v1/users", server.CreateUser)
	r.POST("/api/v1/login", server.Login)
	r.POST("/api/v1/matchups", server.CreateMatchup)
	r.POST("/likes/:id", server.LikeMatchup)
	r.GET("/matchups/:matchup_id/likes", server.GetMatchupLikes)

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

	// Like the matchup
	likeReq, _ := http.NewRequest(http.MethodPost, "/likes/"+strconv.Itoa(matchupID), nil)
	likeReq.Header.Set("Authorization", "Bearer "+token)
	likeW := httptest.NewRecorder()
	r.ServeHTTP(likeW, likeReq)

	// Check if the like was successful
	assert.Equal(t, http.StatusCreated, likeW.Code)

	// Get likes for the matchup using the new endpoint
	getMatchupLikesReq, _ := http.NewRequest(http.MethodGet, "/matchups/"+strconv.Itoa(matchupID)+"/likes", nil)
	getMatchupLikesReq.Header.Set("Authorization", "Bearer "+token)
	getMatchupLikesW := httptest.NewRecorder()
	r.ServeHTTP(getMatchupLikesW, getMatchupLikesReq)

	// Check if the likes were retrieved successfully
	assert.Equal(t, http.StatusOK, getMatchupLikesW.Code)

	// Parse the response to validate likes
	var getMatchupLikesResponse map[string]interface{}
	_ = json.Unmarshal(getMatchupLikesW.Body.Bytes(), &getMatchupLikesResponse)
	likesData, exists := getMatchupLikesResponse["response"].([]interface{})
	if !exists || len(likesData) == 0 {
		t.Fatalf("Matchup likes response does not contain 'response' field or is not an array")
	}

	// Validate the first like in the response
	firstLike := likesData[0].(map[string]interface{})
	assert.Equal(t, float64(userID), firstLike["user_id"])
	assert.Equal(t, float64(matchupID), firstLike["matchup_id"])
}
