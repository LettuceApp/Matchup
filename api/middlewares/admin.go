package middlewares

import (
	httpctx "Matchup/utils/httpctx"
	"net/http"
)

// AdminOnlyMiddleware ensures the authenticated user is an admin.
// Must be used after TokenAuthMiddleware.
func AdminOnlyMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !httpctx.IsAdminRequest(r.Context()) {
				http.Error(w, `{"error":"Forbidden"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
