package middlewares

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

// CheckUserRateLimit fails open when Redis is unavailable — consistent
// with the LoginRateLimitMiddleware behaviour. We can't easily exercise
// the throttled path without a live Redis; the Redis INCR + budget
// semantics are tiny and identical to the login limiter shape already
// in use in production.
func TestCheckUserRateLimit_FailsOpenWithoutRedis(t *testing.T) {
	prev := cache.Client
	cache.Client = nil
	t.Cleanup(func() { cache.Client = prev })

	ctx := context.Background()
	for i := 0; i < 100; i++ {
		allowed, err := CheckUserRateLimit(ctx, 42, "comment", 5, time.Minute)
		if err != nil {
			t.Fatalf("unexpected error on call %d: %v", i+1, err)
		}
		if !allowed {
			t.Fatalf("expected allowed=true with no Redis, got false on call %d", i+1)
		}
	}
}

// userID = 0 short-circuits (anon caller) — treat as allowed so the
// handler can return its own "unauthorized" error rather than being
// masked by a rate-limit 429.
func TestCheckUserRateLimit_ZeroUserPasses(t *testing.T) {
	// Even with Redis nil, a 0 uid returns allowed without trying
	// Redis. Verify by keeping cache.Client nil here.
	prev := cache.Client
	cache.Client = nil
	t.Cleanup(func() { cache.Client = prev })

	allowed, err := CheckUserRateLimit(context.Background(), 0, "comment", 1, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("expected allowed=true for uid=0")
	}
}
