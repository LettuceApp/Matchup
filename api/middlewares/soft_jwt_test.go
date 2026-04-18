package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"Matchup/auth"
	httpctx "Matchup/utils/httpctx"
)

func TestSoftJWTMiddleware(t *testing.T) {
	t.Setenv("API_SECRET", "test-secret")
	middleware := SoftJWTMiddleware()

	t.Run("no token passes through with no user", func(t *testing.T) {
		var called bool
		var gotID uint
		var gotOK bool
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			gotID, gotOK = httpctx.CurrentUserID(r.Context())
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if !called {
			t.Fatal("handler was not called")
		}
		if gotOK {
			t.Errorf("CurrentUserID ok = true, want false")
		}
		if gotID != 0 {
			t.Errorf("CurrentUserID = %d, want 0", gotID)
		}
	})

	t.Run("valid token injects user ID", func(t *testing.T) {
		token, err := auth.CreateToken(99)
		if err != nil {
			t.Fatalf("CreateToken: %v", err)
		}

		var gotID uint
		var gotOK bool
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotID, gotOK = httpctx.CurrentUserID(r.Context())
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if !gotOK {
			t.Fatal("CurrentUserID ok = false, want true")
		}
		if gotID != 99 {
			t.Errorf("CurrentUserID = %d, want 99", gotID)
		}
	})

	t.Run("invalid token still passes through", func(t *testing.T) {
		var called bool
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer invalid.token.here")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if !called {
			t.Fatal("handler was not called for invalid token")
		}
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})
}
