package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"Matchup/cache"
)

// TestLoginRateLimitMiddleware_FailsOpenWithoutRedis verifies that requests
// are allowed through when cache.Client is nil (Redis unavailable). This is
// the documented fail-open behaviour for the Redis-backed rate limiter.
func TestLoginRateLimitMiddleware_FailsOpenWithoutRedis(t *testing.T) {
	prev := cache.Client
	cache.Client = nil
	t.Cleanup(func() { cache.Client = prev })

	handler := LoginRateLimitMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 200; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "192.0.2.1:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 with no Redis, got %d on request %d", w.Code, i+1)
		}
	}
}

func TestGetClientIP_PrefersXForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	if got := getClientIP(req); got != "1.2.3.4" {
		t.Errorf("expected X-Forwarded-For (1.2.3.4), got %q", got)
	}
}

func TestGetClientIP_FallsBackToRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	if got := getClientIP(req); got != "10.0.0.1:1234" {
		t.Errorf("expected RemoteAddr fallback, got %q", got)
	}
}
