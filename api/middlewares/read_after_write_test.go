package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	httpctx "Matchup/utils/httpctx"
)

func TestReadAfterWriteMiddleware(t *testing.T) {
	middleware := ReadAfterWriteMiddleware()

	tests := []struct {
		name       string
		cookie     *http.Cookie
		wantPrimary bool
	}{
		{
			name:        "no cookie",
			cookie:      nil,
			wantPrimary: false,
		},
		{
			name:        "cookie value 1",
			cookie:      &http.Cookie{Name: "_rwp", Value: "1"},
			wantPrimary: true,
		},
		{
			name:        "cookie wrong value",
			cookie:      &http.Cookie{Name: "_rwp", Value: "0"},
			wantPrimary: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPrimary bool
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPrimary = httpctx.ShouldReadPrimary(r.Context())
			}))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if gotPrimary != tt.wantPrimary {
				t.Errorf("ShouldReadPrimary = %v, want %v", gotPrimary, tt.wantPrimary)
			}
		})
	}
}
