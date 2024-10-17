package controllers

import (
	"Matchup/api/middlewares"
)

func (s *Server) initializeRoutes() {
	v1 := s.Router.Group("/api/v1")
	{
		// Login Route
		v1.POST("/login", s.Login)

		// Reset password:
		v1.POST("/password/forgot", s.ForgotPassword)
		v1.POST("/password/reset", s.ResetPassword)

		//Users routes
		v1.POST("/users", s.CreateUser)
		v1.GET("/users", s.GetUsers)
		v1.GET("/users/:id", s.GetUser)
		v1.PUT("/users/:id", middlewares.TokenAuthMiddleware(), s.UpdateUser)
		v1.PUT("/avatar/users/:id", middlewares.TokenAuthMiddleware(), s.UpdateAvatar)
		v1.DELETE("/users/:id", middlewares.TokenAuthMiddleware(), s.DeleteUser)

		//Matchups routes
		v1.POST("/matchups", middlewares.TokenAuthMiddleware(), s.CreateMatchup)
		v1.GET("/matchups", s.GetMatchups)
		v1.GET("/matchups/:id", s.GetMatchup)
		v1.PUT("/matchups/:id", middlewares.TokenAuthMiddleware(), s.UpdateMatchup)
		v1.DELETE("/matchups/:id", middlewares.TokenAuthMiddleware(), s.DeleteMatchup)
		v1.GET("/user_matchups/:id", s.GetUserMatchups)

		// MatchupItem routes
		v1.PUT("/matchup_items/:id/increment_votes", s.IncrementMatchupItemVotes)
		v1.DELETE("/matchup_items/:id", s.DeleteMatchupItem)
		v1.POST("/matchups/:matchup_id/items", s.AddItemToMatchup)

		//Like route
		v1.GET("/likes/matchups/:id", s.GetLikes)
		v1.POST("/likes/:id", middlewares.TokenAuthMiddleware(), s.LikeMatchup)
		v1.DELETE("/likes/:id", middlewares.TokenAuthMiddleware(), s.UnLikeMatchup)
		v1.GET("/matchups/:matchup_id/likes", s.GetMatchupLikes)

		// Comment routes
		v1.POST("/matchups/:matchup_id/comments", middlewares.TokenAuthMiddleware(), s.CreateComment)
		v1.GET("/matchups/:matchup_id/comments", s.GetComments)
		v1.PUT("/comments/:id", middlewares.TokenAuthMiddleware(), s.UpdateComment)
		v1.DELETE("/comments/:id", middlewares.TokenAuthMiddleware(), s.DeleteComment)

	}
}
