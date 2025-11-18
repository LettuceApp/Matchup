package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// helper to reset global state between tests
func resetLimiters() {
	visitorsMu.Lock()
	visitors = make(map[string]*visitor)
	visitorsMu.Unlock()

	loginVisitorsMu.Lock()
	loginVisitors = make(map[string]*visitor)
	loginVisitorsMu.Unlock()
}

// makeTestRouter creates a Gin engine with a single middleware and a test route.
func makeTestRouter(mw gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(mw)
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})
	return r
}

func TestRateLimitMiddleware_AllowsInitialBurst(t *testing.T) {
	resetLimiters()

	// IMPORTANT: call RateLimitMiddleware(), not RateLimitMiddleware
	router := makeTestRouter(RateLimitMiddleware())

	// Should allow at least 5 quick requests (burst size)
	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest(http.MethodGet, "/test", nil)
		// IP won't matter much here as long as it's consistent; gin will use RemoteAddr
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200 on request %d, got %d", i+1, w.Code)
		}
	}

	// 6th request should be rate limited (likely 429)
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429 on 6th request, got %d", w.Code)
	}
}

func TestLoginRateLimitMiddleware_StricterLimit(t *testing.T) {
	resetLimiters()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(LoginRateLimitMiddleware())
	r.POST("/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "login ok"})
	})

	// First 3 attempts should pass (burst = 3)
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest(http.MethodPost, "/login", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200 on login attempt %d, got %d", i+1, w.Code)
		}
	}

	// 4th attempt should be rate limited (429)
	req, _ := http.NewRequest(http.MethodPost, "/login", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429 on 4th login attempt, got %d", w.Code)
	}
}
