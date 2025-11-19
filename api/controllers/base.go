package controllers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"Matchup/api/cache"
	"Matchup/api/middlewares"
	"Matchup/api/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Server struct {
	DB     *gorm.DB
	Router *gin.Engine
}

var errList = make(map[string]string)

func (server *Server) Initialize(DbUser, DbPassword, DbPort, DbHost, DbName string) {
	var dsn string

	// Use Heroku Postgres in production; local DB otherwise
	if strings.EqualFold(os.Getenv("APP_ENV"), "production") {
		dsn = os.Getenv("DATABASE_URL")
		// Heroku Postgres expects SSL
		if dsn != "" && !strings.Contains(dsn, "sslmode=") {
			if strings.Contains(dsn, "?") {
				dsn += "&sslmode=require"
			} else {
				dsn += "?sslmode=require"
			}
		}
	} else {
		// Your original local DSN
		dsn = fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=UTC",
			DbHost, DbUser, DbPassword, DbName, DbPort,
		)
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Cannot connect to Postgres: %v", err)
	}
	server.DB = db

	// Keep your existing migrations exactly as before (uncomment/adjust to match your models)
	if err := server.DB.AutoMigrate(
		&models.User{},
		&models.Matchup{},
		&models.ResetPassword{},
		&models.Like{},
		&models.Comment{},
		&models.MatchupItem{},
	); err != nil {
		log.Fatalf("Error migrating database: %v", err)
	}

	// Initialize Redis cache
	if err := cache.InitFromEnv(); err != nil {
		// For dev, log and continue instead of crashing the app
		log.Printf("warning: could not connect to redis: %v", err)
	}

	if err := promoteSeedAdmin(server.DB); err != nil {
		log.Printf("warning: could not promote default admin: %v", err)
	}

	server.Router = gin.Default()

	// Global middlewares
	server.Router.Use(middlewares.CORSMiddleware())
	server.Router.Use(middlewares.RateLimitMiddleware())

	server.initializeRoutes()

}

func (server *Server) Run(addr string) {
	log.Fatal(http.ListenAndServe(addr, server.Router))
}

func promoteSeedAdmin(db *gorm.DB) error {
	const adminEmail = "cordelljenkins1914@gmail.com"
	if adminEmail == "" {
		return nil
	}

	return db.Model(&models.User{}).
		Where("email = ?", strings.ToLower(strings.TrimSpace(adminEmail))).
		Update("is_admin", true).Error
}
