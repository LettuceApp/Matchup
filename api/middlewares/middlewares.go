package middlewares

import (
	"net/http"
	"os"

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

// This enables us interact with the React Frontend
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		frontend := os.Getenv("FRONTEND_BASE_URL")
		allowedOrigins := []string{
			frontend,
			"http://localhost:3000",
		}

		for _, o := range allowedOrigins {
			if o == origin {
				c.Writer.Header().Set("Access-Control-Allow-Origin", o)
				break
			}
		}

		c.Writer.Header().Set("Vary", "Origin")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers",
			"Content-Type, Authorization, Content-Length, X-CSRF-Token, Accept, Origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods",
			"POST, GET, OPTIONS, PUT, PATCH, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
