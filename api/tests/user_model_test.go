package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"Matchup/controllers"
	"Matchup/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreateUser(t *testing.T) {
	// Set Gin to Test Mode
	gin.SetMode(gin.TestMode)

	// Create a new router
	r := gin.Default()
	server := &controllers.Server{}

	// Use SQLite as an in-memory database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to in-memory database: %v", err)
	}
	server.DB = db
	server.DB.AutoMigrate(&models.User{}) // Ensure the schema is available

	// Define the /users route with the controller function
	r.POST("/api/v1/users", server.CreateUser)

	// Create the mock user request payload
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

	// Record the response
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Check response status code
	assert.Equal(t, http.StatusCreated, w.Code)

	// Parse the response body into a User struct
	var responseBody map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &responseBody)
	if err != nil {
		t.Fatalf("Error unmarshalling response body: %v", err)
	}

	responseUser := responseBody["response"].(map[string]interface{})

	// Check that response contains correct data
	assert.Equal(t, mockUser["username"], responseUser["username"])
	assert.Equal(t, mockUser["email"], responseUser["email"])

	// Password should not be exposed in the response
	_, passwordExists := responseUser["password"]
	assert.False(t, passwordExists, "Password field should not be exposed in response")

}

func TestGetUserByID(t *testing.T) {
	// Set Gin to Test Mode
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	server := &controllers.Server{}

	// Use SQLite as an in-memory database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to in-memory database: %v", err)
	}
	server.DB = db
	server.DB.AutoMigrate(&models.User{})

	// Define the routes with the controller functions
	r.POST("/api/v1/users", server.CreateUser)
	r.GET("/api/v1/users/:id", server.GetUser)

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
	// Print out the response body
	t.Logf("Response body: %s", w.Body.String())

	// Check response status code for creating user
	assert.Equal(t, http.StatusCreated, w.Code)

	// Parse the response body into a map
	var responseBody map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &responseBody)
	if err != nil {
		t.Fatalf("Error unmarshalling response body: %v", err)
	}

	// Extract the "response" field
	responseUser, ok := responseBody["response"].(map[string]interface{})
	if !ok {
		t.Fatalf("Error extracting 'response' from response body")
	}

	// Extract the user ID
	userIDFloat, ok := responseUser["id"].(float64)
	if !ok {
		t.Fatalf("Error extracting 'id' from response data")
	}
	userID := uint(userIDFloat)

	// Create a GET request to retrieve the user by ID
	getReq, err := http.NewRequest(http.MethodGet, "/api/v1/users/"+strconv.Itoa(int(userID)), nil)
	if err != nil {
		t.Fatalf("Error creating HTTP request: %v", err)
	}

	// Record the response for getting the user
	getW := httptest.NewRecorder()
	r.ServeHTTP(getW, getReq)

	// Print out the response body
	t.Logf("Response body: %s", getW.Body.String())

	// Check response status code for getting user
	assert.Equal(t, http.StatusOK, getW.Code)

	// Parse the response body into a map
	var getResponseBody map[string]interface{}
	err = json.Unmarshal(getW.Body.Bytes(), &getResponseBody)
	if err != nil {
		t.Fatalf("Error unmarshalling get user response body: %v", err)
	}

	// Extract the "response" field
	getUserData, ok := getResponseBody["response"].(map[string]interface{})
	if !ok {
		t.Fatalf("Error extracting 'response' from get user response body")
	}

	// Verify the user data
	assert.Equal(t, mockUser["username"], getUserData["username"])
	assert.Equal(t, mockUser["email"], getUserData["email"])

	// Ensure password is not exposed in the response
	_, passwordExists := getUserData["password"]
	assert.False(t, passwordExists, "Password field should not be exposed in response")
}

func TestGetAllUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	server := &controllers.Server{}

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to in-memory database: %v", err)
	}
	server.DB = db
	server.DB.AutoMigrate(&models.User{})

	r.POST("/api/v1/users", server.CreateUser)
	r.GET("/api/v1/users", server.GetUsers)

	// Create multiple mock users
	mockUsers := []map[string]string{
		{"username": "testuser1", "email": "testuser1@example.com", "password": "password123"},
		{"username": "testuser2", "email": "testuser2@example.com", "password": "password123"},
	}

	for _, mockUser := range mockUsers {
		requestBody, err := json.Marshal(mockUser)
		if err != nil {
			t.Fatalf("Error creating request body: %v", err)
		}

		req, err := http.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBuffer(requestBody))
		if err != nil {
			t.Fatalf("Error creating HTTP request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
	}

	// Get all users
	getReq, err := http.NewRequest(http.MethodGet, "/api/v1/users", nil)
	if err != nil {
		t.Fatalf("Error creating HTTP request: %v", err)
	}

	getW := httptest.NewRecorder()
	r.ServeHTTP(getW, getReq)

	assert.Equal(t, http.StatusOK, getW.Code)

	// Parse the response body into a map
	var getUsersResponseBody map[string]interface{}
	err = json.Unmarshal(getW.Body.Bytes(), &getUsersResponseBody)
	if err != nil {
		t.Fatalf("Error unmarshalling response body: %v", err)
	}

	// Extract the "response" field which should contain the list of users
	usersData, ok := getUsersResponseBody["response"].([]interface{})
	if !ok {
		t.Fatalf("Error extracting 'response' field from response body")
	}

	// Verify the number of users returned matches the number of mock users created
	assert.Equal(t, len(mockUsers), len(usersData))

	for _, user := range usersData {
		userMap := user.(map[string]interface{})
		assert.NotEmpty(t, userMap["username"], "Username should be present in response")
		assert.NotEmpty(t, userMap["email"], "Email should be present in response")

		// Password should not be exposed in the response
		_, passwordExists := userMap["password"]
		assert.False(t, passwordExists, "Password field should not be exposed in response")
	}
}

func TestUpdateUser(t *testing.T) {
	// Set Gin to Test Mode
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	server := &controllers.Server{}

	// Use SQLite as an in-memory database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to in-memory database: %v", err)
	}
	server.DB = db
	server.DB.AutoMigrate(&models.User{})

	// Create user
	r.POST("/api/v1/users", server.CreateUser)
	mockUser := map[string]string{
		"username": "testuser",
		"email":    "testuser@example.com",
		"password": "password123",
	}
	requestBody, err := json.Marshal(mockUser)
	if err != nil {
		t.Fatalf("Error creating request body: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBuffer(requestBody))
	if err != nil {
		t.Fatalf("Error creating HTTP request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	// Parse the response body into a map
	var responseBody map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &responseBody)
	if err != nil {
		t.Fatalf("Error unmarshalling response body: %v", err)
	}

	// Extract the "response" field
	responseUser, ok := responseBody["response"].(map[string]interface{})
	if !ok {
		t.Fatalf("Error extracting 'response' from response body")
	}

	// Extract the user ID
	userIDFloat, ok := responseUser["id"].(float64)
	if !ok {
		t.Fatalf("Error extracting 'id' from response data")
	}
	userID := uint(userIDFloat)

	// Set up authenticated router
	authenticatedRouter := gin.Default()
	authenticatedRouter.Use(AuthMiddlewareForTests(userID))
	authenticatedRouter.PUT("/api/v1/users/:id", server.UpdateUser)

	updatedUser := map[string]string{
		"email":            "updateduser@example.com",
		"current_password": "password123",
		"new_password":     "newpassword123",
	}
	updateRequestBody, err := json.Marshal(updatedUser)
	if err != nil {
		t.Fatalf("Error creating update request body: %v", err)
	}

	updateReq, err := http.NewRequest(http.MethodPut, "/api/v1/users/"+strconv.Itoa(int(userID)), bytes.NewBuffer(updateRequestBody))
	if err != nil {
		t.Fatalf("Error creating HTTP request: %v", err)
	}
	updateReq.Header.Set("Content-Type", "application/json")

	updateW := httptest.NewRecorder()
	authenticatedRouter.ServeHTTP(updateW, updateReq)

	assert.Equal(t, http.StatusOK, updateW.Code)

	// Parse the update response body into a map
	var updateResponseBody map[string]interface{}
	err = json.Unmarshal(updateW.Body.Bytes(), &updateResponseBody)
	if err != nil {
		t.Fatalf("Error unmarshalling update response body: %v", err)
	}

	// Extract the "response" field
	updatedUserData, ok := updateResponseBody["response"].(map[string]interface{})
	if !ok {
		t.Fatalf("Error extracting 'response' from update user response body")
	}

	// Verify the updated email
	assert.Equal(t, updatedUser["email"], updatedUserData["email"])

	// Ensure password is not exposed in the response
	_, passwordExists := updatedUserData["password"]
	assert.False(t, passwordExists, "Password field should not be exposed in response")
}

func TestDeleteUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	server := &controllers.Server{}

	// Setup in-memory database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to in-memory database: %v", err)
	}
	server.DB = db
	server.DB.AutoMigrate(&models.User{})

	// Define routes
	r.POST("/api/v1/users", server.CreateUser)
	r.POST("/api/v1/login", server.Login)
	authenticatedRouter := r.Group("/api/v1")
	authenticatedRouter.Use(AuthMiddlewareForTests(1)) // Assume user ID 1 for testing
	authenticatedRouter.DELETE("/users/:id", server.DeleteUser)

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

	req, err := http.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBuffer(requestBody))
	if err != nil {
		t.Fatalf("Error creating HTTP request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Parse the response to get user ID
	var responseBody map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &responseBody)
	if err != nil {
		t.Fatalf("Error unmarshalling response body: %v", err)
	}
	userID := int(responseBody["response"].(map[string]interface{})["id"].(float64))

	// Send request to delete the user
	deleteReq, err := http.NewRequest(http.MethodDelete, "/api/v1/users/"+strconv.Itoa(userID), nil)
	if err != nil {
		t.Fatalf("Error creating HTTP request: %v", err)
	}
	deleteReq.Header.Set("Authorization", "Bearer your_token")

	deleteW := httptest.NewRecorder()
	r.ServeHTTP(deleteW, deleteReq)

	assert.Equal(t, http.StatusOK, deleteW.Code)
}
