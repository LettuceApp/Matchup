package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	httpctx "Matchup/utils/httpctx"
)

func TestAdminOnlyMiddleware(t *testing.T) {
	middleware := AdminOnlyMiddleware()

	t.Run("admin context passes through", func(t *testing.T) {
		var called bool
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		ctx := httpctx.WithIsAdmin(req.Context(), true)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if !called {
			t.Fatal("handler was not called for admin request")
		}
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("non-admin context returns 403", func(t *testing.T) {
		var called bool
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		ctx := httpctx.WithIsAdmin(req.Context(), false)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if called {
			t.Fatal("handler should not be called for non-admin request")
		}
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("no admin flag in context returns 403", func(t *testing.T) {
		var called bool
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if called {
			t.Fatal("handler should not be called when no admin flag is set")
		}
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})
}
