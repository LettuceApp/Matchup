package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func resetLimiters() {
	loginVisitorsMu.Lock()
	loginVisitors = make(map[string]*visitor)
	loginVisitorsMu.Unlock()
}

func TestLoginRateLimitMiddleware_AllowsInitialRequests(t *testing.T) {
	resetLimiters()

	handler := LoginRateLimitMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "192.0.2.1:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 on request %d, got %d", i+1, w.Code)
		}
	}
}

func TestLoginRateLimitMiddleware_BlocksAfterBurst(t *testing.T) {
	resetLimiters()

	const ip = "192.0.2.2:9999"
	handler := LoginRateLimitMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Drain the entire burst (100 tokens).
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 on request %d while draining burst, got %d", i+1, w.Code)
		}
	}

	// The 101st request must be rejected.
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = ip
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 after burst exhausted, got %d", w.Code)
	}
}

func TestLoginRateLimitMiddleware_UsesXForwardedFor(t *testing.T) {
	resetLimiters()

	handler := LoginRateLimitMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// With X-Forwarded-For set, that value should be used as the key.
	// Drain the burst for the forwarded IP.
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		req.Header.Set("X-Forwarded-For", "1.2.3.4")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 on request %d, got %d", i+1, w.Code)
		}
	}

	// A request from a different RemoteAddr but same X-Forwarded-For should be rejected.
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "10.0.0.2:5678"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 for same forwarded IP after burst, got %d", w.Code)
	}
}
