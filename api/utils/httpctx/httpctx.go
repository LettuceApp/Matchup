package httpctx

import "github.com/gin-gonic/gin"

// CurrentUserID retrieves the authenticated user ID from Gin context if present.
func CurrentUserID(c *gin.Context) (uint, bool) {
	val, exists := c.Get("userID")
	if !exists {
		return 0, false
	}
	uid, ok := val.(uint)
	return uid, ok
}

// IsAdminRequest indicates whether the current request is from an admin.
func IsAdminRequest(c *gin.Context) bool {
	val, exists := c.Get("isAdmin")
	if !exists {
		return false
	}
	isAdmin, ok := val.(bool)
	return ok && isAdmin
}
