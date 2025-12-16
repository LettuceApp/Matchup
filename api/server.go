package api

import (
	"log"
	"os"

	"Matchup/controllers"

	"github.com/joho/godotenv"
)

var server = controllers.Server{}

func init() {
	// Only load .env in local dev
	if os.Getenv("APP_ENV") != "production" {
		_ = godotenv.Load()
	}
}

func Run() {
	// Make sure Gin runs in release mode on Render
	if os.Getenv("APP_ENV") == "production" {
		os.Setenv("GIN_MODE", "release")
	}

	// Initialize DB (in prod, base.go uses DATABASE_URL)
	server.Initialize(
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_NAME"),
	)

	// Render provides PORT, fallback for local dev only
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting server on port %s\n", port)

	server.Run(":" + port)
}
