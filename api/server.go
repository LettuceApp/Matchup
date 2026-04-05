package api

import (
	"fmt"
	"log"
	"os"
	"strings"

	"Matchup/controllers"
	appdb "Matchup/db"
	"Matchup/migrations"

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
	// Build DSN for migrations (same logic as base.go)
	var dsn string
	if strings.EqualFold(os.Getenv("APP_ENV"), "production") {
		dsn = os.Getenv("DATABASE_URL")
	} else {
		dsn = fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
			os.Getenv("DB_HOST"), os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"),
			os.Getenv("DB_NAME"), os.Getenv("DB_PORT"),
		)
	}

	// Open connection for Goose migrations
	db, err := appdb.Connect(dsn)
	if err != nil {
		log.Fatalf("Failed to open DB for migrations: %v", err)
	}

	if err := migrations.Run(db.DB); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	db.Close()

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
