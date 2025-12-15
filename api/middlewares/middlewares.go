package middlewares

import (
	"net/http"

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

		origin := c.GetHeader("Origin")

		allowedOrigins := map[string]bool{
			"https://matchup-uud5.onrender.com": true,
			"https://matchup-vhl6.onrender.com": true, // backend URL can call itself
			"http://localhost:3000":             true,
			"http://localhost:5173":             true,
		}

		if allowedOrigins[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}

		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers",
			"Origin, Content-Type, Authorization, Accept")
		c.Header("Access-Control-Allow-Methods",
			"GET, POST, PUT, PATCH, DELETE, OPTIONS")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
