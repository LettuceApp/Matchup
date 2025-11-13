package middlewares

import (
	"net/http"
	"os"
	"strings"

	"Matchup/api/auth"

	"github.com/gin-gonic/gin"
)

func TokenAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := auth.ExtractTokenID(c.Request)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}
		c.Set("userID", userID)
		c.Next()
	}
}

// This enables us interact with the React Frontend
func CORSMiddleware() gin.HandlerFunc {
	// FRONTEND_ORIGINS example:
	// "https://matchup-frontend.netlify.app,https://matchup.com,http://localhost:3000"
	raw := os.Getenv("FRONTEND_ORIGINS")
	parts := strings.Split(raw, ",")
	allowed := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.TrimRight(p, "/")
		if p != "" {
			allowed = append(allowed, p)
		}
	}
	// Developer-friendly default
	if len(allowed) == 0 {
		allowed = []string{"http://localhost:3000"}
	}

	return func(c *gin.Context) {
		origin := strings.TrimRight(c.GetHeader("Origin"), "/")
		allowOrigin := ""
		for _, o := range allowed {
			if strings.EqualFold(o, origin) {
				allowOrigin = o
				break
			}
		}

		// Only emit CORS headers for known origins
		if allowOrigin != "" {
			c.Header("Access-Control-Allow-Origin", allowOrigin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers",
				"Authorization, Content-Type, Content-Length, Accept, Accept-Encoding, X-CSRF-Token, Cache-Control, X-Requested-With")
			// c.Header("Access-Control-Expose-Headers", "Authorization") // if needed
		}

		// Proper preflight handling
		if c.Request.Method == http.MethodOptions {
			if allowOrigin == "" {
				c.AbortWithStatus(http.StatusForbidden) // origin not allowed => fail the preflight clearly
				return
			}
			c.AbortWithStatus(http.StatusNoContent) // 204 with the headers above
			return
		}

		c.Next()
	}
}
