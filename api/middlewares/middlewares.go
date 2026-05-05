package middlewares

import (
	"Matchup/auth"
	httpctx "Matchup/utils/httpctx"
	"net/http"

	"github.com/jmoiron/sqlx"
)

// ReadAfterWriteMiddleware checks for the _rwp (read-write-primary) cookie
// set by write handlers. When present, all reads for that request are routed
// to the primary instead of the replica, avoiding stale-read surprises
// caused by replication lag. The cookie has a 5-second Max-Age — just long
// enough to cover the typical replica lag window.
func ReadAfterWriteMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c, err := r.Cookie("_rwp"); err == nil && c.Value == "1" {
				r = r.WithContext(httpctx.WithReadPrimary(r.Context()))
			}
			next.ServeHTTP(w, r)
		})
	}
}

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
			// `deleted_at IS NULL` shuts out self-deleted + admin-banned
			// users holding a still-valid access token. Their refresh
			// token is already revoked on soft-delete (account_delete.go)
			// so expiring the access token completes the lockout.
			if err := db.GetContext(r.Context(), &user, "SELECT id, is_admin FROM users WHERE id = $1 AND deleted_at IS NULL", userID); err != nil {
				http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
				return
			}

			ctx := httpctx.WithUserID(r.Context(), userID)
			ctx = httpctx.WithIsAdmin(ctx, user.IsAdmin)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
