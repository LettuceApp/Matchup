package controllers

import (
	"Matchup/api/middlewares"
)

func (s *Server) initializeRoutes() {
	v1 := s.Router.Group("/api/v1")
	{
		// ---------- Public routes ----------

		// Auth & password reset (with stricter rate limit)
		v1.POST("/login", middlewares.LoginRateLimitMiddleware(), s.Login)
		v1.POST("/password/forgot", middlewares.LoginRateLimitMiddleware(), s.ForgotPassword)
		v1.POST("/password/reset", middlewares.LoginRateLimitMiddleware(), s.ResetPassword)

		// Users (public)
		v1.POST("/users", s.CreateUser)
		v1.GET("/users", s.GetUsers)
		v1.GET("/users/:id", s.GetUser)

		// Matchups (public read)
		v1.GET("/matchups", s.GetMatchups)
		v1.GET("/matchups/:id", s.GetMatchup)

		// Popular matchups (public read)
		v1.GET("/matchups/popular", s.GetPopularMatchups)

		// User matchups (public read)
		v1.GET("/users/:id/matchups", s.GetUserMatchups)
		v1.GET("/users/:id/matchups/:matchupid", s.GetUserMatchup)

		// Matchup items - public voting
		v1.PATCH("/matchup_items/:id/vote", s.IncrementMatchupItemVotes)

		// Comments - public read
		// 🔴 IMPORTANT: use :id here, not :matchup_id
		v1.GET("/matchups/:id/comments", s.GetComments)

		// Likes - public read
		v1.GET("/matchups/:id/likes", s.GetLikes)
		v1.GET("/users/:id/likes", s.GetUserLikes)

		// ---------- Auth-protected routes ----------
		auth := v1.Group("")
		auth.Use(middlewares.TokenAuthMiddleware(s.DB))

		// Current user
		auth.GET("/me", s.GetCurrentUser)

		// Users - protected modifications
		auth.PUT("/users/:id", s.UpdateUser)
		auth.PUT("/users/:id/avatar", s.UpdateAvatar)
		auth.DELETE("/users/:id", s.DeleteUser)

		// Matchups - create & modify
		auth.POST("/users/:id/matchups", s.CreateMatchup)
		auth.PUT("/matchups/:id", s.UpdateMatchup)
		auth.DELETE("/matchups/:id", s.DeleteMatchup)

		// Matchup items - protected modifications
		auth.DELETE("/matchup_items/:id", s.DeleteMatchupItem)
		auth.PUT("/matchup_items/:id", s.UpdateMatchupItem)
		auth.POST("/matchups/:id/items", s.AddItemToMatchup)

		// Likes - protected create/delete
		auth.POST("/matchups/:id/likes", s.LikeMatchup)
		auth.DELETE("/matchups/:id/likes", s.UnLikeMatchup)

		// Comments - protected create/update/delete
		// 🔴 ALSO use :id here
		auth.POST("/matchups/:id/comments", s.CreateComment)
		auth.PUT("/comments/:id", s.UpdateComment)
		auth.DELETE("/comments/:id", s.DeleteComment)

		// ---------- Admin routes ----------
		admin := auth.Group("/admin")
		admin.Use(middlewares.AdminOnlyMiddleware())
		admin.GET("/users", s.AdminListUsers)
		admin.PATCH("/users/:id/role", s.AdminUpdateUserRole)
		admin.GET("/matchups", s.AdminListMatchups)
		admin.DELETE("/matchups/:id", s.AdminDeleteMatchup)
	}
}
