package main

import (
	api "Matchup"
)

// @title Matchup API
// @version 1.0
// @description API for brackets, matchups, voting, and engagement
// @BasePath /api/v1
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Provide a valid JWT as: Bearer <token>
func main() {
	api.Run()
}
