package share

import (
	"context"
	"errors"
	"fmt"
	"time"

	"Matchup/cache"

	"github.com/redis/go-redis/v9"
)

// Cache TTLs. Positive cache is long because the cache key is
// versioned by updated_at — stale entries simply don't get hit. The
// negative cache is short because a matchup going from missing → exists
// happens in seconds (typical Create flow).
const (
	positiveTTL = 7 * 24 * time.Hour
	negativeTTL = 60 * time.Second
)

// ErrCacheMiss is returned by Get when no entry exists. Distinct from
// a real error so callers can `errors.Is` against it without stringy
// checks.
var ErrCacheMiss = errors.New("share: cache miss")

// ErrNegativeCacheHit is returned when the key has been recorded as
// "known 404" — callers should skip DB lookup and return 404 directly.
// Absorbs crawler re-try storms on deleted content.
var ErrNegativeCacheHit = errors.New("share: negative cache hit")

// cacheKey builds the Redis key for a rendered OG PNG. Keyed by
// (kind, short_id, updated_at_unix) so edits invalidate automatically
// without a manual counter.
func cacheKey(kind ContentKind, shortID string, updatedAtUnix int64) string {
	return fmt.Sprintf("og:%s:%s:%d", kind, shortID, updatedAtUnix)
}

// missKey builds the negative-cache key for a short_id. No version —
// "I tried to find this and it didn't exist" applies regardless of
// when someone last edited something.
func missKey(kind ContentKind, shortID string) string {
	return fmt.Sprintf("og:miss:%s:%s", kind, shortID)
}

// Get returns cached PNG bytes if present. Returns ErrCacheMiss or
// ErrNegativeCacheHit in the two known-absent cases, other errors
// otherwise (which callers should log-and-ignore — cache failures
// should never block a render).
func Get(ctx context.Context, kind ContentKind, shortID string, updatedAtUnix int64) ([]byte, error) {
	if cache.Client == nil {
		return nil, ErrCacheMiss
	}
	// Negative cache first — a repeated 404 hammer shouldn't do two
	// GETs per attempt.
	if _, err := cache.Client.Get(ctx, missKey(kind, shortID)).Result(); err == nil {
		return nil, ErrNegativeCacheHit
	} else if !errors.Is(err, redis.Nil) {
		return nil, err
	}
	b, err := cache.Client.Get(ctx, cacheKey(kind, shortID, updatedAtUnix)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrCacheMiss
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Set stores PNG bytes under the versioned cache key with the positive
// TTL. No-op if Redis is unavailable — caller should not care.
func Set(ctx context.Context, kind ContentKind, shortID string, updatedAtUnix int64, png []byte) error {
	if cache.Client == nil {
		return nil
	}
	return cache.Client.Set(ctx, cacheKey(kind, shortID, updatedAtUnix), png, positiveTTL).Err()
}

// SetMiss records a negative-cache entry for a known-missing short_id.
// Absorbs crawler re-fetch loops on deleted or never-existed content.
func SetMiss(ctx context.Context, kind ContentKind, shortID string) error {
	if cache.Client == nil {
		return nil
	}
	return cache.Client.Set(ctx, missKey(kind, shortID), "1", negativeTTL).Err()
}
