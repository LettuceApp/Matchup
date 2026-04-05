package middlewares

import (
	"Matchup/auth"
	httpctx "Matchup/utils/httpctx"
	"net/http"

	"github.com/jmoiron/sqlx"
)

// SoftJWTMiddleware tries to extract a JWT and inject the user ID into context, but never rejects.
// Used on public routes that want optional viewer context (e.g., for showing like/follow state).
func SoftJWTMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if uid, err := auth.ExtractTokenID(r); err == nil {
				r = r.WithContext(httpctx.WithUserID(r.Context(), uid))
			}
			next.ServeHTTP(w, r)
		})
	}
}

// TokenAuthMiddleware validates the JWT token and injects userID + isAdmin into the request context.
func TokenAuthMiddleware(db *sqlx.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, err := auth.ExtractTokenID(r)
			if err != nil {
				http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
				return
			}

			var user struct {
				ID      uint `db:"id"`
				IsAdmin bool `db:"is_admin"`
			}
			if err := db.GetContext(r.Context(), &user, "SELECT id, is_admin FROM users WHERE id = $1", userID); err != nil {
				http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
				return
			}

			ctx := httpctx.WithUserID(r.Context(), userID)
			ctx = httpctx.WithIsAdmin(ctx, user.IsAdmin)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
