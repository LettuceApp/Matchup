package controllers

import (
	"fmt"
	"log"
	"net/http"

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

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", DbHost, DbPort, DbUser, DbPassword, DbName)
	var err error
	server.DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}

	err = server.DB.AutoMigrate(
		&models.User{},
		&models.Matchup{},
		&models.ResetPassword{},
		&models.Like{},
		&models.Comment{},
		&models.MatchupItem{},
	)
	if err != nil {
		log.Fatalf("Error migrating database: %v", err)
	}

	server.Router = gin.Default()
	server.Router.Use(middlewares.CORSMiddleware())

	server.initializeRoutes()

}

func (server *Server) Run(addr string) {
	log.Fatal(http.ListenAndServe(addr, server.Router))
}
