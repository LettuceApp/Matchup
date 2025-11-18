package middlewares

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// visitor holds the rate limiter and the last time we saw this IP.
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

var (
	// General API visitors
	visitors   = make(map[string]*visitor)
	visitorsMu sync.Mutex

	// Stricter visitors for login / password reset
	loginVisitors   = make(map[string]*visitor)
	loginVisitorsMu sync.Mutex
)

// newVisitorLimiter creates a new limiter for general API calls.
// Example: 1 request/second average, burst of 5.
func newVisitorLimiter() *rate.Limiter {
	return rate.NewLimiter(rate.Every(time.Second), 100)
}

// newLoginVisitorLimiter creates a stricter limiter for auth-sensitive routes.
// Example: 1 request every 10 seconds on average, burst of 3.
func newLoginVisitorLimiter() *rate.Limiter {
	return rate.NewLimiter(rate.Every(10*time.Second), 100)
}

func getVisitor(ip string) *rate.Limiter {
	visitorsMu.Lock()
	defer visitorsMu.Unlock()

	v, exists := visitors[ip]
	if !exists {
		limiter := newVisitorLimiter()
		visitors[ip] = &visitor{
			limiter:  limiter,
			lastSeen: time.Now(),
		}
		return limiter
	}

	v.lastSeen = time.Now()
	return v.limiter
}

func getLoginVisitor(ip string) *rate.Limiter {
	loginVisitorsMu.Lock()
	defer loginVisitorsMu.Unlock()

	v, exists := loginVisitors[ip]
	if !exists {
		limiter := newLoginVisitorLimiter()
		loginVisitors[ip] = &visitor{
			limiter:  limiter,
			lastSeen: time.Now(),
		}
		return limiter
	}

	v.lastSeen = time.Now()
	return v.limiter
}

// RateLimitMiddleware applies a simple per-IP rate limit for all routes.
func RateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		limiter := getVisitor(ip)

		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "Too many requests. Please slow down.",
			})
			return
		}

		c.Next()
	}
}

// LoginRateLimitMiddleware applies a stricter per-IP rate limit for auth routes.
func LoginRateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		limiter := getLoginVisitor(ip)

		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "Too many authentication attempts. Please wait and try again.",
			})
			return
		}

		c.Next()
	}
}
