package middlewares

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// AdminOnlyMiddleware ensures that the incoming request is authenticated and belongs to an admin user.
func AdminOnlyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		isAdmin, _ := c.Get("isAdmin")
		if adminFlag, ok := isAdmin.(bool); ok && adminFlag {
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
	}
}
