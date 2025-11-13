package controllers

import (
	"Matchup/api/middlewares"

	"github.com/gin-gonic/gin"
)

func (s *Server) initializeRoutes() {

	s.Router.GET("/", func(c *gin.Context) {
		c.Redirect(302, "https://matchup-frontend-5a27300cbe76.herokuapp.com/")
	})

	v1 := s.Router.Group("/api/v1")
	{
		// Users routes
		v1.POST("/login", s.Login)
		v1.POST("/password/forgot", s.ForgotPassword)
		v1.POST("/password/reset", s.ResetPassword)
		v1.POST("/users", s.CreateUser)
		v1.GET("/users", s.GetUsers)
		v1.GET("/users/:id", s.GetUser)
		v1.PUT("/users/:id", middlewares.TokenAuthMiddleware(), s.UpdateUser)
		v1.PUT("/users/:id/avatar", middlewares.TokenAuthMiddleware(), s.UpdateAvatar)
		v1.DELETE("/users/:id", middlewares.TokenAuthMiddleware(), s.DeleteUser)

		// Matchup routes
		// FIX: add leading slash so the route becomes /api/v1/users/:id/create-matchup
		v1.POST("/users/:id/create-matchup", middlewares.TokenAuthMiddleware(), s.CreateMatchup)
		v1.GET("/matchups", s.GetMatchups)
		v1.GET("/matchup/:id", s.GetMatchup) // singular form
		v1.PUT("/matchup/:id", middlewares.TokenAuthMiddleware(), s.UpdateMatchup)
		v1.DELETE("/matchup/:id", middlewares.TokenAuthMiddleware(), s.DeleteMatchup)

		// User matchups route
		v1.GET("/users/:id/matchups", s.GetUserMatchups)
		v1.GET("/users/:id/matchups/:matchupid", s.GetUserMatchup)

		// MatchupItem routes
		v1.PATCH("/matchup_items/:id/vote", s.IncrementMatchupItemVotes)
		v1.DELETE("/matchup_items/:id", middlewares.TokenAuthMiddleware(), s.DeleteMatchupItem)
		v1.PUT("/matchup_items/:id", middlewares.TokenAuthMiddleware(), s.UpdateMatchupItem)
		v1.POST("/matchups/:matchup_id/items", middlewares.TokenAuthMiddleware(), s.AddItemToMatchup)

		// Like routes
		v1.GET("/likes/matchups/:id", s.GetLikes)
		v1.POST("/likes/matchups/:id", middlewares.TokenAuthMiddleware(), s.LikeMatchup)
		v1.DELETE("/likes/matchups/:id", middlewares.TokenAuthMiddleware(), s.UnLikeMatchup)
		v1.GET("/users/:id/likes", s.GetUserLikes)

		// Comments routes
		v1.POST("/matchups/:matchup_id/comments", middlewares.TokenAuthMiddleware(), s.CreateComment)
		v1.GET("/matchups/:matchup_id/comments", s.GetComments)
		v1.PUT("/comments/:id", middlewares.TokenAuthMiddleware(), s.UpdateComment)
		v1.DELETE("/comments/:id", middlewares.TokenAuthMiddleware(), s.DeleteComment)
	}
}
