package middlewares

import (
	"net/http"
	"strings"

	"Matchup/auth"
	"Matchup/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TokenAuthMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := auth.ExtractTokenID(c.Request)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}

		var user models.User
		if err := db.Select("id", "is_admin").First(&user, userID).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}

		c.Set("userID", userID)
		c.Set("isAdmin", user.IsAdmin)
		c.Next()
	}
}

func CORSMiddleware() gin.HandlerFunc {
	allowedOrigins := []string{
		"https://matchup-uud5.onrender.com", // frontend
		"https://matchup-vh16.onrender.com", // backend (for redirects, health checks)
		"http://localhost:3000",
		"http://localhost:8888",
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// If origin is allowed, set headers
		for _, o := range allowedOrigins {
			if strings.EqualFold(o, origin) {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Set("Vary", "Origin")
				break
			}
		}

		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers",
			"Content-Type, Authorization, X-CSRF-Token, X-Requested-With, Accept")
		c.Writer.Header().Set("Access-Control-Allow-Methods",
			"GET, POST, PUT, PATCH, DELETE, OPTIONS")

		// Respond immediately to preflight
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
