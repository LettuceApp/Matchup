package middlewares

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// visitor holds the rate limiter and the last time we saw this IP.
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

var (
	loginVisitors   = make(map[string]*visitor)
	loginVisitorsMu sync.Mutex
)

func newLoginVisitorLimiter() *rate.Limiter {
	return rate.NewLimiter(rate.Every(10*time.Second), 100)
}

func getLoginVisitor(ip string) *rate.Limiter {
	loginVisitorsMu.Lock()
	defer loginVisitorsMu.Unlock()

	v, exists := loginVisitors[ip]
	if !exists {
		limiter := newLoginVisitorLimiter()
		loginVisitors[ip] = &visitor{limiter: limiter, lastSeen: time.Now()}
		return limiter
	}
	v.lastSeen = time.Now()
	return v.limiter
}

// LoginRateLimitMiddleware applies a stricter per-IP rate limit for auth routes.
func LoginRateLimitMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
				ip = xff
			}
			if !getLoginVisitor(ip).Allow() {
				http.Error(w, `{"error":"Too many authentication attempts. Please wait and try again."}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
