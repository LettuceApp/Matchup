package controllers

import (
	"Matchup/api/auth"
	"Matchup/api/middlewares"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *Server) initializeRoutes() {

	s.Router.GET("/", func(c *gin.Context) {
		if target := s.frontendRedirectTarget(c); target != "" {
			c.Redirect(http.StatusFound, target)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "matchup-api",
		})
	})

	s.Router.GET("/login", func(c *gin.Context) {
		if target := frontendLoginURL(); target != "" {
			c.Redirect(http.StatusFound, target)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "matchup-api",
			"hint":    "set FRONTEND_LOGIN_URL or FRONTEND_BASE_URL",
		})
	})

	v1 := s.Router.Group("/api/v1")
	{
		// Users routes
		v1.GET("/me", middlewares.TokenAuthMiddleware(), s.GetCurrentUser)
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

	// Optional: JSON 404 so you see exactly what method/path missed
	s.Router.NoRoute(func(c *gin.Context) {
		c.JSON(404, gin.H{
			"error":  "route not found",
			"method": c.Request.Method,
			"path":   c.Request.URL.Path,
		})
	})
}

func (s *Server) frontendRedirectTarget(c *gin.Context) string {
	if home := frontendHomeURL(); home != "" {
		if _, err := auth.ExtractTokenID(c.Request); err == nil {
			return home
		}
	}
	return frontendLoginURL()
}

func frontendLoginURL() string {
	if login := normalizeURL(os.Getenv("FRONTEND_LOGIN_URL")); login != "" {
		return login
	}
	if base := normalizeURL(os.Getenv("FRONTEND_BASE_URL")); base != "" {
		return base + "/login"
	}
	if app := normalizeAppURL(); app != "" {
		return app + "/login"
	}
	return ""
}

func frontendHomeURL() string {
	if home := normalizeURL(os.Getenv("FRONTEND_HOME_URL")); home != "" {
		return home
	}
	if base := normalizeURL(os.Getenv("FRONTEND_BASE_URL")); base != "" {
		return base + "/home"
	}
	if app := normalizeAppURL(); app != "" {
		return app + "/home"
	}
	return ""
}

func normalizeAppURL() string {
	app := strings.TrimSpace(os.Getenv("APP_BASE_URL"))
	if app == "" {
		return ""
	}
	lower := strings.ToLower(app)
	if strings.Contains(lower, "amazonaws.com") || strings.Contains(lower, "s3.") {
		return ""
	}
	return normalizeURL(app)
}

func normalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	lower := strings.ToLower(raw)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		scheme := "https://"
		if strings.HasPrefix(lower, "localhost") || strings.HasPrefix(lower, "127.0.0.1") {
			scheme = "http://"
		}
		raw = scheme + raw
	}

	return strings.TrimRight(raw, "/")
}
