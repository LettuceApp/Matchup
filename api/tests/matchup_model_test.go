package tests

import (
	"bytes"
	"encoding/json"
	"html"
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

// AuthMiddlewareForTests simulates an authenticated user by setting userID in the context
func AuthMiddlewareForTests(userID uint) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("userID", userID)
		c.Next()
	}
}

func TestCreateMatchup(t *testing.T) {
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

	// Ensure the schema is available for related tables (including comments)
	err = server.DB.AutoMigrate(&models.User{}, &models.Matchup{}, &models.MatchupItem{}, &models.Comment{})
	if err != nil {
		t.Fatalf("Failed to migrate tables: %v", err)
	}

	// Define the routes with the controller functions
	r.POST("/api/v1/users", server.CreateUser)
	r.POST("/api/v1/login", server.Login)
	r.POST("/api/v1/matchups", server.CreateMatchup)

	// Create a mock user request payload
	mockUser := map[string]string{
		"username": "testuser",
		"email":    "testuser@example.com",
		"password": "password123",
	}
	requestBody, err := json.Marshal(mockUser)
	if err != nil {
		t.Fatalf("Error creating request body: %v", err)
	}

	// Create an HTTP request with the mock user
	req, err := http.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBuffer(requestBody))
	if err != nil {
		t.Fatalf("Error creating HTTP request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Record the response for creating the user
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Check response status code for creating user
	assert.Equal(t, http.StatusCreated, w.Code)

	// Parse the response body to get the user ID
	var responseBody map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &responseBody)
	if err != nil {
		t.Fatalf("Error unmarshalling response body: %v", err)
	}

	responseUser := responseBody["response"].(map[string]interface{})
	userID := uint(responseUser["id"].(float64)) // Extract the user ID

	// Login to get the token
	loginPayload := map[string]string{
		"email":    mockUser["email"],
		"password": mockUser["password"],
	}
	loginRequestBody, err := json.Marshal(loginPayload)
	if err != nil {
		t.Fatalf("Error creating login request body: %v", err)
	}

	loginReq, err := http.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewBuffer(loginRequestBody))
	if err != nil {
		t.Fatalf("Error creating login HTTP request: %v", err)
	}
	loginReq.Header.Set("Content-Type", "application/json")

	// Record the response for login
	loginW := httptest.NewRecorder()
	r.ServeHTTP(loginW, loginReq)

	// Check response status code for login
	assert.Equal(t, http.StatusOK, loginW.Code)

	// Parse the login response body to get the token
	var loginResponseBody map[string]interface{}
	err = json.Unmarshal(loginW.Body.Bytes(), &loginResponseBody)
	if err != nil {
		t.Fatalf("Error unmarshalling login response body: %v", err)
	}

	token := loginResponseBody["response"].(map[string]interface{})["token"].(string)

	// Create a matchup with the uint user ID and without comments initially
	mockMatchup := map[string]interface{}{
		"title":     "Test Matchup",
		"content":   "This is a test matchup",
		"author_id": userID, // Use the uint user ID here
		"items": []map[string]interface{}{
			{"item": "Item 1", "votes": 0},
			{"item": "Item 2", "votes": 0},
		},
	}

	matchupRequestBody, err := json.Marshal(mockMatchup)
	if err != nil {
		t.Fatalf("Error creating matchup request body: %v", err)
	}

	matchupReq, err := http.NewRequest(http.MethodPost, "/api/v1/matchups", bytes.NewBuffer(matchupRequestBody))
	if err != nil {
		t.Fatalf("Error creating matchup HTTP request: %v", err)
	}
	matchupReq.Header.Set("Content-Type", "application/json")
	matchupReq.Header.Set("Authorization", "Bearer "+token)

	// Record the response for creating the matchup
	matchupW := httptest.NewRecorder()
	r.ServeHTTP(matchupW, matchupReq)

	// Check response status code for creating matchup
	assert.Equal(t, http.StatusCreated, matchupW.Code)

	// Parse the response body to verify the creation
	var matchupResponse map[string]interface{}
	err = json.Unmarshal(matchupW.Body.Bytes(), &matchupResponse)
	if err != nil {
		t.Fatalf("Error unmarshalling matchup response body: %v", err)
	}

	// Validate that the "response" field exists in the JSON
	response, exists := matchupResponse["response"].(map[string]interface{})
	if !exists {
		t.Fatalf("Matchup response does not contain 'response' field")
	}

	// Validate fields in the response
	assert.Equal(t, mockMatchup["title"], response["title"])
	assert.Equal(t, mockMatchup["content"], response["content"])
	assert.Equal(t, float64(userID), response["author_id"]) // Note: JSON unmarshalling gives float64 for numbers
}

func TestGetMatchupByID(t *testing.T) {
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

	// Ensure the schema is available for related tables (including comments)
	err = server.DB.AutoMigrate(&models.User{}, &models.Matchup{}, &models.MatchupItem{}, &models.Comment{})
	if err != nil {
		t.Fatalf("Failed to migrate tables: %v", err)
	}

	// Define the routes with the controller functions
	r.POST("/api/v1/users", server.CreateUser)
	r.POST("/api/v1/login", server.Login)
	r.POST("/api/v1/matchups", server.CreateMatchup)
	r.GET("/api/v1/matchups/:id", server.GetMatchup)

	// Create a mock user
	mockUser := map[string]string{
		"username": "testuser",
		"email":    "testuser@example.com",
		"password": "password123",
	}
	requestBody, err := json.Marshal(mockUser)
	if err != nil {
		t.Fatalf("Error creating request body: %v", err)
	}

	// Create the user
	req, err := http.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBuffer(requestBody))
	if err != nil {
		t.Fatalf("Error creating HTTP request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	// Parse user ID
	var responseBody map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &responseBody)
	if err != nil {
		t.Fatalf("Error unmarshalling response body: %v", err)
	}
	responseUser := responseBody["response"].(map[string]interface{})
	userID := uint(responseUser["id"].(float64))

	// Login to get the token
	loginPayload := map[string]string{
		"email":    mockUser["email"],
		"password": mockUser["password"],
	}
	loginRequestBody, err := json.Marshal(loginPayload)
	if err != nil {
		t.Fatalf("Error creating login request body: %v", err)
	}
	loginReq, err := http.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewBuffer(loginRequestBody))
	if err != nil {
		t.Fatalf("Error creating login HTTP request: %v", err)
	}
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	r.ServeHTTP(loginW, loginReq)
	assert.Equal(t, http.StatusOK, loginW.Code)
	var loginResponseBody map[string]interface{}
	err = json.Unmarshal(loginW.Body.Bytes(), &loginResponseBody)
	if err != nil {
		t.Fatalf("Error unmarshalling login response body: %v", err)
	}
	token := loginResponseBody["response"].(map[string]interface{})["token"].(string)

	// Create a matchup
	mockMatchup := map[string]interface{}{
		"title":     "Test Matchup",
		"content":   "This is a test matchup",
		"author_id": userID,
		"items": []map[string]interface{}{
			{"item": "Item 1", "votes": 0},
			{"item": "Item 2", "votes": 0},
		},
	}
	matchupRequestBody, err := json.Marshal(mockMatchup)
	if err != nil {
		t.Fatalf("Error creating matchup request body: %v", err)
	}
	matchupReq, err := http.NewRequest(http.MethodPost, "/api/v1/matchups", bytes.NewBuffer(matchupRequestBody))
	if err != nil {
		t.Fatalf("Error creating matchup HTTP request: %v", err)
	}
	matchupReq.Header.Set("Content-Type", "application/json")
	matchupReq.Header.Set("Authorization", "Bearer "+token)
	matchupW := httptest.NewRecorder()
	r.ServeHTTP(matchupW, matchupReq)
	assert.Equal(t, http.StatusCreated, matchupW.Code)

	// Parse the matchup ID
	var matchupResponseBody map[string]interface{}
	err = json.Unmarshal(matchupW.Body.Bytes(), &matchupResponseBody)
	if err != nil {
		t.Fatalf("Error unmarshalling matchup response body: %v", err)
	}
	matchupResponse := matchupResponseBody["response"].(map[string]interface{})
	matchupID := uint(matchupResponse["id"].(float64))

	// Get the matchup by ID
	getReq, err := http.NewRequest(http.MethodGet, "/api/v1/matchups/"+strconv.Itoa(int(matchupID)), nil)
	if err != nil {
		t.Fatalf("Error creating HTTP request: %v", err)
	}
	getW := httptest.NewRecorder()
	r.ServeHTTP(getW, getReq)
	assert.Equal(t, http.StatusOK, getW.Code)

	// Parse the response
	var getMatchupResponseBody map[string]interface{}
	err = json.Unmarshal(getW.Body.Bytes(), &getMatchupResponseBody)
	if err != nil {
		t.Fatalf("Error unmarshalling get matchup response body: %v", err)
	}
	// Verify the matchup data
	assert.Equal(t, matchupResponse["title"], getMatchupResponseBody["title"])
	assert.Equal(t, matchupResponse["content"], getMatchupResponseBody["content"])
}

func TestGetAllMatchups(t *testing.T) {
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

	// Ensure the schema is available for related tables (including comments)
	err = server.DB.AutoMigrate(&models.User{}, &models.Matchup{}, &models.MatchupItem{}, &models.Comment{})
	if err != nil {
		t.Fatalf("Failed to migrate tables: %v", err)
	}

	// Define the routes with the controller functions
	r.POST("/api/v1/users", server.CreateUser)
	r.POST("/api/v1/login", server.Login)
	r.POST("/api/v1/matchups", server.CreateMatchup)
	r.GET("/api/v1/matchups", server.GetMatchups)

	// Create a mock user
	mockUser := map[string]string{
		"username": "testuser",
		"email":    "testuser@example.com",
		"password": "password123",
	}
	requestBody, err := json.Marshal(mockUser)
	if err != nil {
		t.Fatalf("Error creating request body: %v", err)
	}

	// Create the user
	req, err := http.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBuffer(requestBody))
	if err != nil {
		t.Fatalf("Error creating HTTP request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	// Parse user ID
	var responseBody map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &responseBody)
	if err != nil {
		t.Fatalf("Error unmarshalling response body: %v", err)
	}
	responseUser := responseBody["response"].(map[string]interface{})
	userID := uint(responseUser["id"].(float64))

	// Login to get the token
	loginPayload := map[string]string{
		"email":    mockUser["email"],
		"password": mockUser["password"],
	}
	loginRequestBody, err := json.Marshal(loginPayload)
	if err != nil {
		t.Fatalf("Error creating login request body: %v", err)
	}
	loginReq, err := http.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewBuffer(loginRequestBody))
	if err != nil {
		t.Fatalf("Error creating login HTTP request: %v", err)
	}
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	r.ServeHTTP(loginW, loginReq)
	assert.Equal(t, http.StatusOK, loginW.Code)
	var loginResponseBody map[string]interface{}
	err = json.Unmarshal(loginW.Body.Bytes(), &loginResponseBody)
	if err != nil {
		t.Fatalf("Error unmarshalling login response body: %v", err)
	}
	token := loginResponseBody["response"].(map[string]interface{})["token"].(string)

	// Create a matchup
	mockMatchup := map[string]interface{}{
		"title":     "Test Matchup",
		"content":   "This is a test matchup",
		"author_id": userID,
		"items": []map[string]interface{}{
			{"item": "Item 1", "votes": 0},
			{"item": "Item 2", "votes": 0},
		},
	}
	matchupRequestBody, err := json.Marshal(mockMatchup)
	if err != nil {
		t.Fatalf("Error creating matchup request body: %v", err)
	}
	matchupReq, err := http.NewRequest(http.MethodPost, "/api/v1/matchups", bytes.NewBuffer(matchupRequestBody))
	if err != nil {
		t.Fatalf("Error creating matchup HTTP request: %v", err)
	}
	matchupReq.Header.Set("Content-Type", "application/json")
	matchupReq.Header.Set("Authorization", "Bearer "+token)
	matchupW := httptest.NewRecorder()
	r.ServeHTTP(matchupW, matchupReq)
	assert.Equal(t, http.StatusCreated, matchupW.Code)

	// Get all matchups
	getReq, err := http.NewRequest(http.MethodGet, "/api/v1/matchups", nil)
	if err != nil {
		t.Fatalf("Error creating HTTP request: %v", err)
	}
	getW := httptest.NewRecorder()
	r.ServeHTTP(getW, getReq)
	assert.Equal(t, http.StatusOK, getW.Code)

	// Parse the response
	var getMatchupsResponse []map[string]interface{}
	err = json.Unmarshal(getW.Body.Bytes(), &getMatchupsResponse)
	if err != nil {
		t.Fatalf("Error unmarshalling get matchups response body: %v", err)
	}

	// Verify that at least one matchup exists
	assert.NotEmpty(t, getMatchupsResponse, "Expected at least one matchup in the response")

	// Check the data of the first matchup
	firstMatchup := getMatchupsResponse[0]
	assert.Equal(t, mockMatchup["title"], firstMatchup["title"])
	assert.Equal(t, mockMatchup["content"], firstMatchup["content"])
}

func TestGetUserMatchups(t *testing.T) {
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

	// Ensure the schema is available for related tables (including comments)
	err = server.DB.AutoMigrate(&models.User{}, &models.Matchup{}, &models.MatchupItem{}, &models.Comment{})
	if err != nil {
		t.Fatalf("Failed to migrate tables: %v", err)
	}

	// Define the routes with the controller functions
	r.POST("/api/v1/users", server.CreateUser)
	r.POST("/api/v1/login", server.Login)
	r.POST("/api/v1/matchups", server.CreateMatchup)
	r.GET("/api/v1/users/:id/matchups", server.GetUserMatchups)

	// Create a mock user
	mockUser := map[string]string{
		"username": "testuser",
		"email":    "testuser@example.com",
		"password": "password123",
	}
	requestBody, err := json.Marshal(mockUser)
	if err != nil {
		t.Fatalf("Error creating request body: %v", err)
	}

	// Create the user
	req, err := http.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBuffer(requestBody))
	if err != nil {
		t.Fatalf("Error creating HTTP request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	// Parse user ID
	var responseBody map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &responseBody)
	if err != nil {
		t.Fatalf("Error unmarshalling response body: %v", err)
	}
	responseUser := responseBody["response"].(map[string]interface{})
	userID := uint(responseUser["id"].(float64))

	// Login to get the token
	loginPayload := map[string]string{
		"email":    mockUser["email"],
		"password": mockUser["password"],
	}
	loginRequestBody, err := json.Marshal(loginPayload)
	if err != nil {
		t.Fatalf("Error creating login request body: %v", err)
	}
	loginReq, err := http.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewBuffer(loginRequestBody))
	if err != nil {
		t.Fatalf("Error creating login HTTP request: %v", err)
	}
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	r.ServeHTTP(loginW, loginReq)
	assert.Equal(t, http.StatusOK, loginW.Code)
	var loginResponseBody map[string]interface{}
	err = json.Unmarshal(loginW.Body.Bytes(), &loginResponseBody)
	if err != nil {
		t.Fatalf("Error unmarshalling login response body: %v", err)
	}
	token := loginResponseBody["response"].(map[string]interface{})["token"].(string)

	// Create a matchup
	mockMatchup := map[string]interface{}{
		"title":     "User's Matchup",
		"content":   "This is a matchup by the user",
		"author_id": userID,
		"items": []map[string]interface{}{
			{"item": "Item A", "votes": 0},
			{"item": "Item B", "votes": 0},
		},
	}
	matchupRequestBody, err := json.Marshal(mockMatchup)
	if err != nil {
		t.Fatalf("Error creating matchup request body: %v", err)
	}
	matchupReq, err := http.NewRequest(http.MethodPost, "/api/v1/matchups", bytes.NewBuffer(matchupRequestBody))
	if err != nil {
		t.Fatalf("Error creating matchup HTTP request: %v", err)
	}
	matchupReq.Header.Set("Content-Type", "application/json")
	matchupReq.Header.Set("Authorization", "Bearer "+token)
	matchupW := httptest.NewRecorder()
	r.ServeHTTP(matchupW, matchupReq)
	assert.Equal(t, http.StatusCreated, matchupW.Code)

	// Get matchups by user ID
	getReq, err := http.NewRequest(http.MethodGet, "/api/v1/users/"+strconv.Itoa(int(userID))+"/matchups", nil)
	if err != nil {
		t.Fatalf("Error creating HTTP request: %v", err)
	}
	getW := httptest.NewRecorder()
	r.ServeHTTP(getW, getReq)
	assert.Equal(t, http.StatusOK, getW.Code)

	// Parse the response
	var userMatchupsResponse []map[string]interface{}
	err = json.Unmarshal(getW.Body.Bytes(), &userMatchupsResponse)
	if err != nil {
		t.Fatalf("Error unmarshalling user matchups response body: %v", err)
	}

	// Verify that the matchup exists
	assert.NotEmpty(t, userMatchupsResponse, "Expected at least one matchup for the user")
	firstMatchup := userMatchupsResponse[0]

	// Decode the HTML entities in the title and content before comparing
	decodedTitle := html.UnescapeString(firstMatchup["title"].(string))
	decodedContent := html.UnescapeString(firstMatchup["content"].(string))

	assert.Equal(t, mockMatchup["title"], decodedTitle)
	assert.Equal(t, mockMatchup["content"], decodedContent)
	assert.Equal(t, float64(userID), firstMatchup["author_id"])
}

func TestUpdateMatchup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Set up router and server
	r := gin.Default()
	server := &controllers.Server{}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to in-memory database: %v", err)
	}
	server.DB = db
	server.DB.AutoMigrate(&models.User{}, &models.Matchup{}, &models.MatchupItem{}, &models.Comment{})

	r.POST("/api/v1/users", server.CreateUser)
	r.POST("/api/v1/login", server.Login)
	r.POST("/api/v1/matchups", server.CreateMatchup)

	// Set up a router with the auth middleware for updating matchups
	authenticatedRouter := gin.Default()
	authenticatedRouter.Use(AuthMiddlewareForTests(1)) // Use 1 assuming this is the user ID after creation
	authenticatedRouter.PUT("/api/v1/matchups/:id", server.UpdateMatchup)

	// Create and authenticate a user
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
	var responseBody map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	responseUser := responseBody["response"].(map[string]interface{})
	userID := uint(responseUser["id"].(float64))

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
	json.Unmarshal(loginW.Body.Bytes(), &loginResponseBody)
	token := loginResponseBody["response"].(map[string]interface{})["token"].(string)

	// Create a matchup
	mockMatchup := map[string]interface{}{
		"title":     "Initial Title",
		"content":   "Initial Content",
		"author_id": userID,
		"items": []map[string]interface{}{
			{"item": "Item A", "votes": 0},
			{"item": "Item B", "votes": 0},
		},
	}
	matchupRequestBody, _ := json.Marshal(mockMatchup)
	matchupReq, _ := http.NewRequest(http.MethodPost, "/api/v1/matchups", bytes.NewBuffer(matchupRequestBody))
	matchupReq.Header.Set("Content-Type", "application/json")
	matchupReq.Header.Set("Authorization", "Bearer "+token)
	matchupW := httptest.NewRecorder()
	r.ServeHTTP(matchupW, matchupReq)
	var matchupResponse map[string]interface{}
	json.Unmarshal(matchupW.Body.Bytes(), &matchupResponse)
	matchupID := uint(matchupResponse["response"].(map[string]interface{})["id"].(float64))

	// Update the matchup
	updatedMatchup := map[string]string{
		"title":   "Updated Title",
		"content": "Updated Content",
	}
	updateRequestBody, _ := json.Marshal(updatedMatchup)
	updateReq, _ := http.NewRequest(http.MethodPut, "/api/v1/matchups/"+strconv.Itoa(int(matchupID)), bytes.NewBuffer(updateRequestBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("Authorization", "Bearer "+token)

	updateW := httptest.NewRecorder()
	authenticatedRouter.ServeHTTP(updateW, updateReq)

	// Check status code and response
	assert.Equal(t, http.StatusOK, updateW.Code)
	var updatedResponse map[string]interface{}
	err = json.Unmarshal(updateW.Body.Bytes(), &updatedResponse)
	if err != nil {
		t.Fatalf("Error unmarshalling update response body: %v", err)
	}

	response, exists := updatedResponse["response"].(map[string]interface{})
	if !exists {
		t.Fatalf("Updated matchup response does not contain 'response' field")
	}

	// Validate fields in the response
	assert.Equal(t, "Updated Title", response["title"])
	assert.Equal(t, "Updated Content", response["content"])
}

func TestIncrementMatchupItemVotes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Set up router and server
	r := gin.Default()
	server := &controllers.Server{}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to in-memory database: %v", err)
	}
	server.DB = db
	server.DB.AutoMigrate(&models.User{}, &models.Matchup{}, &models.MatchupItem{}, &models.Comment{})

	r.POST("/api/v1/users", server.CreateUser)
	r.POST("/api/v1/login", server.Login)
	r.POST("/api/v1/matchups", server.CreateMatchup)
	r.PUT("/api/v1/matchups/:id/items/:item_id/vote", server.IncrementMatchupItemVotes)

	// Create and authenticate a user
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
	var responseBody map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	responseUser := responseBody["response"].(map[string]interface{})
	userID := uint(responseUser["id"].(float64))

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
	json.Unmarshal(loginW.Body.Bytes(), &loginResponseBody)
	token := loginResponseBody["response"].(map[string]interface{})["token"].(string)

	// Create a matchup
	mockMatchup := map[string]interface{}{
		"title":     "Matchup for Voting",
		"content":   "Content for Voting",
		"author_id": userID,
		"items": []map[string]interface{}{
			{"item": "Votable Item", "votes": 0},
		},
	}
	matchupRequestBody, _ := json.Marshal(mockMatchup)
	matchupReq, _ := http.NewRequest(http.MethodPost, "/api/v1/matchups", bytes.NewBuffer(matchupRequestBody))
	matchupReq.Header.Set("Content-Type", "application/json")
	matchupReq.Header.Set("Authorization", "Bearer "+token)
	matchupW := httptest.NewRecorder()
	r.ServeHTTP(matchupW, matchupReq)
	var matchupResponse map[string]interface{}
	json.Unmarshal(matchupW.Body.Bytes(), &matchupResponse)
	matchupID := uint(matchupResponse["response"].(map[string]interface{})["id"].(float64))
	itemID := uint(matchupResponse["response"].(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["id"].(float64))

	// Increment vote for the item
	incrementReq, _ := http.NewRequest(http.MethodPut, "/api/v1/matchups/"+strconv.Itoa(int(matchupID))+"/items/"+strconv.Itoa(int(itemID))+"/vote", nil)
	incrementReq.Header.Set("Content-Type", "application/json")
	incrementReq.Header.Set("Authorization", "Bearer "+token)
	incrementW := httptest.NewRecorder()
	r.ServeHTTP(incrementW, incrementReq)

	// Check the response
	assert.Equal(t, http.StatusOK, incrementW.Code)
	var incrementResponse map[string]interface{}
	json.Unmarshal(incrementW.Body.Bytes(), &incrementResponse)
	item := incrementResponse["response"].(map[string]interface{})
	assert.Equal(t, float64(1), item["votes"])
}
func TestAddItemToMatchup(t *testing.T) {
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

	// Ensure the schema is available for related tables (including comments)
	err = server.DB.AutoMigrate(&models.User{}, &models.Matchup{}, &models.MatchupItem{}, &models.Comment{})
	if err != nil {
		t.Fatalf("Failed to migrate tables: %v", err)
	}

	// Middleware to mock user authentication for testing
	authMiddleware := func(c *gin.Context) {
		c.Set("userID", uint(1)) // Assuming the user ID is 1 for testing purposes
		c.Next()
	}

	// Define the routes with the controller functions
	r.POST("/api/v1/users", server.CreateUser)
	r.POST("/api/v1/login", server.Login)
	r.POST("/api/v1/matchups", server.CreateMatchup)

	// Applying the middleware to the authenticated group
	authenticatedRouter := r.Group("/api/v1")
	authenticatedRouter.Use(authMiddleware)
	authenticatedRouter.POST("/matchups/:matchup_id/items", server.AddItemToMatchup)

	// Create a mock user request payload
	mockUser := map[string]string{
		"username": "testuser",
		"email":    "testuser@example.com",
		"password": "password123",
	}
	requestBody, err := json.Marshal(mockUser)
	if err != nil {
		t.Fatalf("Error creating request body: %v", err)
	}

	// Create an HTTP request with the mock user
	req, err := http.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBuffer(requestBody))
	if err != nil {
		t.Fatalf("Error creating HTTP request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Record the response for creating the user
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Check response status code for creating user
	assert.Equal(t, http.StatusCreated, w.Code)

	// Parse the response body to get the user ID
	var responseBody map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &responseBody)
	if err != nil {
		t.Fatalf("Error unmarshalling response body: %v", err)
	}

	responseUser := responseBody["response"].(map[string]interface{})
	userID := uint(responseUser["id"].(float64)) // Extract the user ID

	// Login to get the token
	loginPayload := map[string]string{
		"email":    mockUser["email"],
		"password": mockUser["password"],
	}
	loginRequestBody, err := json.Marshal(loginPayload)
	if err != nil {
		t.Fatalf("Error creating login request body: %v", err)
	}

	loginReq, err := http.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewBuffer(loginRequestBody))
	if err != nil {
		t.Fatalf("Error creating login HTTP request: %v", err)
	}
	loginReq.Header.Set("Content-Type", "application/json")

	// Record the response for login
	loginW := httptest.NewRecorder()
	r.ServeHTTP(loginW, loginReq)

	// Check response status code for login
	assert.Equal(t, http.StatusOK, loginW.Code)

	// Parse the login response body to get the token
	var loginResponseBody map[string]interface{}
	err = json.Unmarshal(loginW.Body.Bytes(), &loginResponseBody)
	if err != nil {
		t.Fatalf("Error unmarshalling login response body: %v", err)
	}

	token := loginResponseBody["response"].(map[string]interface{})["token"].(string)

	// Create a matchup with the uint user ID and without comments initially
	mockMatchup := map[string]interface{}{
		"title":     "Test Matchup",
		"content":   "This is a test matchup",
		"author_id": userID, // Use the uint user ID here
		"items": []map[string]interface{}{
			{"item": "Initial Item", "votes": 0},
		},
	}

	matchupRequestBody, err := json.Marshal(mockMatchup)
	if err != nil {
		t.Fatalf("Error creating matchup request body: %v", err)
	}

	matchupReq, err := http.NewRequest(http.MethodPost, "/api/v1/matchups", bytes.NewBuffer(matchupRequestBody))
	if err != nil {
		t.Fatalf("Error creating matchup HTTP request: %v", err)
	}
	matchupReq.Header.Set("Content-Type", "application/json")
	matchupReq.Header.Set("Authorization", "Bearer "+token)

	// Record the response for creating the matchup
	matchupW := httptest.NewRecorder()
	r.ServeHTTP(matchupW, matchupReq)

	// Check response status code for creating matchup
	assert.Equal(t, http.StatusCreated, matchupW.Code)

	// Parse the response body to get the matchup ID
	var matchupResponse map[string]interface{}
	err = json.Unmarshal(matchupW.Body.Bytes(), &matchupResponse)
	if err != nil {
		t.Fatalf("Error unmarshalling matchup response body: %v", err)
	}

	matchupData := matchupResponse["response"].(map[string]interface{})
	matchupID := uint(matchupData["id"].(float64))

	// Add a new item to the matchup
	mockItem := map[string]interface{}{
		"item":  "New Item",
		"votes": 0,
	}
	itemRequestBody, err := json.Marshal(mockItem)
	if err != nil {
		t.Fatalf("Error creating item request body: %v", err)
	}

	addItemReq, err := http.NewRequest(http.MethodPost, "/api/v1/matchups/"+strconv.Itoa(int(matchupID))+"/items", bytes.NewBuffer(itemRequestBody))
	if err != nil {
		t.Fatalf("Error creating add item HTTP request: %v", err)
	}
	addItemReq.Header.Set("Content-Type", "application/json")
	addItemReq.Header.Set("Authorization", "Bearer "+token)

	// Record the response for adding the item
	addItemW := httptest.NewRecorder()
	r.ServeHTTP(addItemW, addItemReq)

	// Check response status code for adding item
	assert.Equal(t, http.StatusCreated, addItemW.Code)

	// Parse the response body to verify the new item
	var itemResponse map[string]interface{}
	err = json.Unmarshal(addItemW.Body.Bytes(), &itemResponse)
	if err != nil {
		t.Fatalf("Error unmarshalling item response body: %v", err)
	}

	// Validate the fields directly without assuming a nested "response" field
	assert.Equal(t, mockItem["item"], itemResponse["item"])
	assert.Equal(t, float64(mockItem["votes"].(int)), itemResponse["votes"].(float64))
}

func TestDeleteMatchupWithItems(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	server := &controllers.Server{}

	// Setup in-memory database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to in-memory database: %v", err)
	}
	server.DB = db
	server.DB.AutoMigrate(&models.User{}, &models.Matchup{}, &models.MatchupItem{}, &models.Comment{})

	// Define routes
	r.POST("/api/v1/users", server.CreateUser)
	r.POST("/api/v1/login", server.Login)
	r.POST("/api/v1/matchups", server.CreateMatchup)

	// Apply the AuthMiddlewareForTests middleware and define authenticated routes
	authenticatedRouter := r.Group("/api/v1")
	authenticatedRouter.Use(AuthMiddlewareForTests(1))
	authenticatedRouter.POST("/matchups/:matchup_id/items", server.AddItemToMatchup)
	authenticatedRouter.DELETE("/matchups/:id", server.DeleteMatchup)
	authenticatedRouter.DELETE("/matchups/items/:id", server.DeleteMatchupItem)

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

	// Check if the matchup was created successfully
	assert.Equal(t, http.StatusCreated, matchupW.Code)

	// Parse the matchup response to get the matchup ID
	var matchupResponse map[string]interface{}
	err = json.Unmarshal(matchupW.Body.Bytes(), &matchupResponse)
	if err != nil {
		t.Fatalf("Error unmarshalling matchup response: %v", err)
	}
	matchupData, exists := matchupResponse["response"].(map[string]interface{})
	if !exists {
		t.Fatalf("Matchup response does not contain 'response' field")
	}
	matchupID := int(matchupData["id"].(float64))

	// Add items to the matchup
	mockItem := map[string]interface{}{
		"item":  "Item 1",
		"votes": 0,
	}
	itemRequestBody, _ := json.Marshal(mockItem)
	itemReq, _ := http.NewRequest(http.MethodPost, "/api/v1/matchups/"+strconv.Itoa(matchupID)+"/items", bytes.NewBuffer(itemRequestBody))
	itemReq.Header.Set("Content-Type", "application/json")
	itemReq.Header.Set("Authorization", "Bearer "+token)
	itemW := httptest.NewRecorder()
	r.ServeHTTP(itemW, itemReq)

	// Check if the item was created successfully
	assert.Equal(t, http.StatusCreated, itemW.Code)

	// Parse the item response to get the item ID
	var itemResponse map[string]interface{}
	err = json.Unmarshal(itemW.Body.Bytes(), &itemResponse)
	if err != nil {
		t.Fatalf("Error unmarshalling item response: %v", err)
	}
	itemData, exists := itemResponse["id"].(float64)
	if !exists {
		t.Fatalf("Item response does not contain 'id' field")
	}
	itemID := int(itemData)

	// Delete the item
	deleteItemReq, _ := http.NewRequest(http.MethodDelete, "/api/v1/matchups/items/"+strconv.Itoa(itemID), nil)
	deleteItemReq.Header.Set("Authorization", "Bearer "+token)
	deleteItemW := httptest.NewRecorder()
	r.ServeHTTP(deleteItemW, deleteItemReq)

	assert.Equal(t, http.StatusOK, deleteItemW.Code)

	// Parse the delete item response
	var deleteItemResponse map[string]interface{}
	_ = json.Unmarshal(deleteItemW.Body.Bytes(), &deleteItemResponse)
	assert.Equal(t, "Item deleted", deleteItemResponse["message"])

	// Delete the matchup
	deleteReq, _ := http.NewRequest(http.MethodDelete, "/api/v1/matchups/"+strconv.Itoa(matchupID), nil)
	deleteReq.Header.Set("Authorization", "Bearer "+token)
	deleteW := httptest.NewRecorder()
	r.ServeHTTP(deleteW, deleteReq)

	assert.Equal(t, http.StatusOK, deleteW.Code)

	// Parse the delete matchup response
	var deleteResponse map[string]interface{}
	_ = json.Unmarshal(deleteW.Body.Bytes(), &deleteResponse)
	assert.Equal(t, "Matchup deleted", deleteResponse["message"])
}
