package controllers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"Matchup/cache"
	"Matchup/middlewares"
	"Matchup/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Server struct {
	DB     *gorm.DB
	Router *gin.Engine
}

// ===============================
// SECURE ADMIN SEEDING
// ===============================
func seedAdmin(db *gorm.DB) error {
	adminEmail := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL")))
	adminPassword := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD"))

	// If environment vars aren't provided, do NOTHING.
	if adminEmail == "" || adminPassword == "" {
		log.Println("[seedAdmin] ADMIN_EMAIL or ADMIN_PASSWORD not set — skipping admin creation.")
		return nil
	}

	var existing models.User
	err := db.Where("email = ?", adminEmail).First(&existing).Error

	// If admin does not exist, create them
	if errors.Is(err, gorm.ErrRecordNotFound) {
		log.Println("[seedAdmin] Creating initial admin:", adminEmail)

		admin := models.User{
			Username: strings.Split(adminEmail, "@")[0],
			Email:    adminEmail,
			Password: adminPassword,
			IsAdmin:  true,
		}

		admin.Prepare()

		if msgs := admin.Validate(""); len(msgs) > 0 {
			log.Printf("[seedAdmin] validation failed: %+v\n", msgs)
			return nil
		}

		_, err = admin.SaveUser(db)
		if err != nil {
			log.Printf("[seedAdmin] failed to create admin: %v\n", err)
			return err
		}

		return nil
	}

	// If admin exists, ensure they stay admin
	if err == nil && !existing.IsAdmin {
		log.Println("[seedAdmin] Ensuring admin flag is set for:", adminEmail)
		return db.Model(&existing).Update("is_admin", true).Error
	}

	return err
}

var errList = make(map[string]string)

// ===============================
// SERVER INITIALIZATION
// ===============================
func (server *Server) Initialize(DbUser, DbPassword, DbPort, DbHost, DbName string) {
	var dsn string

	if strings.EqualFold(os.Getenv("APP_ENV"), "production") {
		dsn = os.Getenv("DATABASE_URL")
		if dsn != "" && !strings.Contains(dsn, "sslmode=") {
			if strings.Contains(dsn, "?") {
				dsn += "&sslmode=require"
			} else {
				dsn += "?sslmode=require"
			}
		}
	} else {
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

	// Auto Migrations
	if err := server.DB.AutoMigrate(
		&models.User{},
		&models.Matchup{},
		&models.ResetPassword{},
		&models.Like{},
		&models.MatchupVote{},
		&models.MatchupVoteRollup{},
		&models.BracketLike{},
		&models.Comment{},
		&models.MatchupItem{},
		&models.Bracket{},
	); err != nil {
		log.Fatalf("Error migrating database: %v", err)
	}

	// Redis init (safe failure)
	if err := cache.InitFromEnv(); err != nil {
		log.Printf("warning: could not connect to redis: %v", err)
	}

	// SECURE ADMIN CREATION
	if err := seedAdmin(server.DB); err != nil {
		log.Printf("error seeding admin user: %v\n", err)
	}

	server.Router = gin.Default()
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
