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
	// Comma-separated list, e.g.:
	// FRONTEND_ORIGINS=https://matchup-frontend.netlify.app,https://matchup.com,http://localhost:3000
	allowed := strings.Split(os.Getenv("FRONTEND_ORIGINS"), ",")
	for i := range allowed {
		allowed[i] = strings.TrimSpace(allowed[i])
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowOrigin := ""

		// If no env is set, fall back to common dev origin (keeps local dev easy)
		if len(allowed) == 1 && allowed[0] == "" {
			allowed = []string{"http://localhost:3000"}
		}

		for _, o := range allowed {
			if o != "" && strings.EqualFold(o, origin) {
				allowOrigin = o
				break
			}
		}

		// Set CORS headers only when origin is explicitly allowed
		if allowOrigin != "" {
			c.Header("Access-Control-Allow-Origin", allowOrigin)
			c.Header("Vary", "Origin") // important for proxies/caches
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers",
				"Authorization, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Accept, Origin, Cache-Control, X-Requested-With")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			// (optional) expose headers if you return auth-related headers
			// c.Header("Access-Control-Expose-Headers", "Authorization")
		}

		// Short-circuit preflight
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
