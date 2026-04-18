package middlewares

import (
	"fmt"
	"net/http"
	"time"

	"Matchup/cache"
)

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	return r.RemoteAddr
}

// LoginRateLimitMiddleware applies a per-IP rate limit for auth routes using Redis.
// Allows 100 requests per 10-second window per IP. Falls back to allowing the
// request if Redis is unavailable.
func LoginRateLimitMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cache.Client == nil {
				next.ServeHTTP(w, r)
				return
			}

			ip := getClientIP(r)
			key := fmt.Sprintf("rate_limit_login:%s", ip)

			count, err := cache.Client.Incr(r.Context(), key).Result()
			if err != nil {
				next.ServeHTTP(w, r) // fail open
				return
			}
			if count == 1 {
				cache.Client.Expire(r.Context(), key, 10*time.Second)
			}
			if count > 100 {
				http.Error(w, `{"error":"Too many authentication attempts. Please wait and try again."}`, http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
