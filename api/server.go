package api

import (
	"fmt"
	"os"
	"strings"

	"Matchup/controllers"

	"github.com/joho/godotenv"
)

var server = controllers.Server{}

func init() {
	// Load .env only outside production. On Heroku, config comes from Config Vars.
	if os.Getenv("APP_ENV") != "production" {
		_ = godotenv.Load()
	}
}

func Run() {
	// Local convenience: try loading .env again (no-op in prod).
	_ = godotenv.Load()

	// Initialize DB using your existing initializer.
	// In prod, base.go will use DATABASE_URL; in dev, it will use these pieces.
	server.Initialize(
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_NAME"),
	)

	port := os.Getenv("PORT")
	if port == "" {
		port = os.Getenv("API_PORT")
		if port == "" {
			port = "8888"
		}
	}

	addr := ":" + strings.TrimSpace(port)
	fmt.Printf("Listening on %s\n", addr)
	server.Run(addr)
}
