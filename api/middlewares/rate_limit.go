package middlewares

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"Matchup/cache"
)

// CheckUserRateLimit is a per-user, Redis-backed INCR/EXPIRE rate limit
// callable from inside a Connect RPC handler. Connect mounts a single
// handler for every method on a service, so chi-level middleware can't
// throttle individual methods (e.g. CreateComment) without touching
// reads on the same service. This helper lets the handler opt in at
// the top of the method.
//
//	allowed, err := CheckUserRateLimit(ctx, uid, "comment", 10, time.Minute)
//	if err == nil && !allowed { return connect.NewError(CodeResourceExhausted, ...) }
//
// Semantics:
//   - bucket — a short namespace ("comment", "vote", etc.) mixed into
//     the Redis key so multiple limits coexist per user.
//   - budget — max operations allowed per window (inclusive).
//   - window — the rolling time window.
//   - Fails OPEN (returns true, nil) when cache.Client is nil or
//     Redis errors. Matches LoginRateLimitMiddleware's behavior: an
//     ops outage shouldn't look like an attack to the user.
//
// The nil-error return when blocked makes callsites easy:
//
//	if ok, _ := CheckUserRateLimit(...); !ok { return over-limit error }
func CheckUserRateLimit(ctx context.Context, userID uint, bucket string, budget int64, window time.Duration) (bool, error) {
	if cache.Client == nil || userID == 0 {
		return true, nil
	}

	key := fmt.Sprintf("rate_limit_user:%s:%d", bucket, userID)
	count, err := cache.Client.Incr(ctx, key).Result()
	if err != nil {
		return true, err // fail-open on Redis failure
	}
	if count == 1 {
		// Set the TTL only on the first increment in the window; every
		// subsequent INCR preserves it. Matches the login limiter.
		cache.Client.Expire(ctx, key, window)
	}
	if count > budget {
		return false, nil
	}
	return true, nil
}

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	return r.RemoteAddr
}

// CheckIPRateLimit is the IP-keyed sibling of CheckUserRateLimit — same
// shape, same fail-open semantics, but the Redis key derives from a
// hashed client IP instead of a user ID. Used to back-stop anonymous
// vote churn: a single machine can't bypass the per-anon-id cap by
// clearing localStorage and looping if the IP-side limiter caps the
// rate at the network layer.
//
// `ip` should be the request's RealIP (chi's middleware.RealIP already
// normalises X-Forwarded-For + X-Real-IP). We hash it before keying so
// raw IPs never end up in Redis logs / scrub-incidents.
//
// Empty IP → fails OPEN. The handler can't meaningfully limit something
// it can't identify.
func CheckIPRateLimit(ctx context.Context, ip, bucket string, budget int64, window time.Duration) (bool, error) {
	if cache.Client == nil {
		return true, nil
	}
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return true, nil
	}
	hashed := hashIPForRateLimit(ip)
	key := fmt.Sprintf("rate_limit_ip:%s:%s", bucket, hashed)
	count, err := cache.Client.Incr(ctx, key).Result()
	if err != nil {
		return true, err // fail-open on Redis failure
	}
	if count == 1 {
		cache.Client.Expire(ctx, key, window)
	}
	return count <= budget, nil
}

// hashIPForRateLimit returns the first 16 hex chars of SHA-256(ip).
// Stable + non-reversible. Truncating keeps the Redis key short
// without meaningfully shrinking the keyspace for our scale.
func hashIPForRateLimit(ip string) string {
	sum := sha256.Sum256([]byte(ip))
	return hex.EncodeToString(sum[:8])
}

// LoginRateLimitMiddleware applies a per-IP rate limit for auth routes using Redis.
// Allows 100 requests per 10-second window per IP. Falls back to allowing the
// request if Redis is unavailable.
func LoginRateLimitMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cache.Client == nil {
				next.ServeHTTP(w, r)
				return
			}

			ip := getClientIP(r)
			key := fmt.Sprintf("rate_limit_login:%s", ip)

			count, err := cache.Client.Incr(r.Context(), key).Result()
			if err != nil {
				next.ServeHTTP(w, r) // fail open
				return
			}
			if count == 1 {
				cache.Client.Expire(r.Context(), key, 10*time.Second)
			}
			if count > 100 {
				http.Error(w, `{"error":"Too many authentication attempts. Please wait and try again."}`, http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
