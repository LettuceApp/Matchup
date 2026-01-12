package controllers

import (
	"Matchup/middlewares"

	"github.com/gin-gonic/gin"
)

func (s *Server) initializeRoutes() {
	v1 := s.Router.Group("/api/v1")
	{

		// Health check
		v1.GET("/", func(c *gin.Context) {
			c.JSON(200, gin.H{"status": "backend running"})
		})

		// ---------- Public routes ----------

		// Auth
		v1.POST("/login", middlewares.LoginRateLimitMiddleware(), s.Login)
		v1.POST("/password/forgot", middlewares.LoginRateLimitMiddleware(), s.ForgotPassword)
		v1.POST("/password/reset", middlewares.LoginRateLimitMiddleware(), s.ResetPassword)

		// Users
		v1.POST("/users", s.CreateUser)
		v1.GET("/users", s.GetUsers)
		v1.GET("/users/:id", s.GetUser)

		// Matchups (public read)
		v1.GET("/matchups", s.GetMatchups)
		v1.GET("/matchups/:id", s.GetMatchup)
		v1.GET("/matchups/popular", s.GetPopularMatchups)
		v1.GET("/home", s.GetHomeSummary)

		// User matchups (public read)
		v1.GET("/users/:id/matchups", s.GetUserMatchups)
		v1.GET("/users/:id/matchups/:matchupid", s.GetUserMatchup)

		// 🔹 Brackets (public read)
		v1.GET("/brackets/popular", s.GetPopularBrackets)
		v1.GET("/brackets/:id", s.GetBracket)
		v1.GET("/brackets/:id/summary", s.GetBracketSummary)
		v1.GET("/users/:id/brackets", s.GetUserBrackets)
		v1.GET("/brackets/:id/matchups", s.GetBracketMatchups)
		v1.GET("/brackets/:id/comments", s.GetBracketComments)

		// Comments (public read)
		v1.GET("/matchups/:id/comments", s.GetComments)

		// Likes (public read)
		v1.GET("/matchups/:id/likes", s.GetLikes)
		v1.GET("/users/:id/likes", s.GetUserLikes)
		v1.GET("/brackets/:id/likes", s.GetBracketLikes)
		v1.GET("/users/:id/bracket_likes", s.GetUserBracketLikes)
		v1.GET("/users/:id/matchup_votes", s.GetUserMatchupVotes)

		// ---------- Auth-protected routes ----------
		auth := v1.Group("")
		auth.Use(middlewares.TokenAuthMiddleware(s.DB))

		// Current user
		auth.GET("/me", s.GetCurrentUser)

		// Users
		auth.PUT("/users/:id", s.UpdateUser)
		auth.PUT("/users/:id/avatar", s.UpdateAvatar)
		auth.DELETE("/users/:id", s.DeleteUser)

		// Matchups
		auth.POST("/users/:id/matchups", s.CreateMatchup)
		auth.PUT("/matchups/:id", s.UpdateMatchup)
		auth.DELETE("/matchups/:id", s.DeleteMatchup)
		auth.POST("/matchups/:id/override-winner", s.OverrideMatchupWinner)
		auth.POST("/matchups/:id/complete", s.CompleteMatchup)
		auth.POST("/matchups/:id/ready", s.ReadyUpMatchup)
		auth.POST("/matchups/:id/activate", s.ActivateMatchup)

		// 🔹 Brackets (create & modify)
		auth.POST("/users/:id/brackets", s.CreateBracket)
		auth.PUT("/brackets/:id", s.UpdateBracket)
		auth.DELETE("/brackets/:id", s.DeleteBracket)

		// Matchup items
		auth.POST("/matchups/:id/items", s.AddItemToMatchup)
		auth.PUT("/matchup_items/:id", s.UpdateMatchupItem)
		auth.DELETE("/matchup_items/:id", s.DeleteMatchupItem)
		auth.PATCH("/matchup_items/:id/vote", s.IncrementMatchupItemVotes)

		// Likes
		auth.POST("/matchups/:id/likes", s.LikeMatchup)
		auth.DELETE("/matchups/:id/likes", s.UnLikeMatchup)
		auth.POST("/brackets/:id/likes", s.LikeBracket)
		auth.DELETE("/brackets/:id/likes", s.UnLikeBracket)
		auth.POST("/brackets/:id/comments", s.CreateBracketComment)
		auth.DELETE("/bracket_comments/:id", s.DeleteBracketComment)

		// Comments
		auth.POST("/matchups/:id/comments", s.CreateComment)
		auth.PUT("/comments/:id", s.UpdateComment)
		auth.DELETE("/comments/:id", s.DeleteComment)

		// Bracket → Matchups
		auth.POST("/brackets/:id/matchups", s.AttachMatchupToBracket)
		auth.DELETE("/brackets/matchups/:matchup_id", s.DetachMatchupFromBracket)
		auth.POST("/brackets/:id/advance", s.AdvanceBracket)
		auth.POST("/internal/brackets/:id/advance", s.InternalAdvanceBracket)
		auth.POST("/matchups/:id/resolve-and-advance", s.ResolveTieAndAdvance)

		// ---------- Admin routes ----------
		admin := auth.Group("/admin")
		admin.Use(middlewares.AdminOnlyMiddleware())

		admin.GET("/users", s.AdminListUsers)
		admin.PATCH("/users/:id/role", s.AdminUpdateUserRole)
		admin.DELETE("/users/:id", s.AdminDeleteUser)
		admin.GET("/matchups", s.AdminListMatchups)
		admin.DELETE("/matchups/:id", s.AdminDeleteMatchup)
		admin.GET("/brackets", s.AdminListBrackets)
		admin.DELETE("/brackets/:id", s.AdminDeleteBracket)
	}
}
