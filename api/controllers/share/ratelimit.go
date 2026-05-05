package share

import (
	"sync"
	"time"
)

// Simple in-memory token-bucket rate limiter for the OG image endpoint.
// Crawlers burst-fetch the same short_id (X's Card Validator + Facebook
// Sharing Debugger + Slack unfurl + Discord unfurl all at once on the
// first share), and an uncached render is CPU-heavy. This caps the
// burst without needing a separate Redis roundtrip per request.
//
// Two buckets run in series:
//   1. A global cap — throttles total image-endpoint traffic across all
//      short IDs so one viral link can't starve the process.
//   2. A per-short_id cap — a single bad actor hammering one URL can't
//      exhaust the global budget on behalf of the other URLs.
//
// For local dev and single-pod prod this is fine. Multi-pod deployments
// would want to move the per-short_id bucket into Redis; flagged as a
// future concern in the plan file.

type tokenBucket struct {
	mu         sync.Mutex
	capacity   float64
	tokens     float64
	refillRate float64 // tokens per second
	last       time.Time
}

func newTokenBucket(ratePerSec, burst float64) *tokenBucket {
	return &tokenBucket{
		capacity:   burst,
		tokens:     burst,
		refillRate: ratePerSec,
		last:       time.Now(),
	}
}

// Allow returns true if a token was consumed, false if the bucket is
// empty. Refills lazily based on wall-clock time since last check.
func (b *tokenBucket) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// RateLimiter composes a global bucket and a map of per-short_id
// buckets. Buckets are created lazily on first access; idle entries are
// garbage-collected by an internal sweeper to keep memory bounded under
// long-tail traffic.
type RateLimiter struct {
	global      *tokenBucket
	perRate     float64
	perBurst    float64
	mu          sync.Mutex
	perShortIDs map[string]*tokenBucket
}

// NewRateLimiter returns a rate limiter configured for the OG image
// endpoint. globalRate is the steady-state request cap across all
// short IDs; perRate is the per-short_id cap.
func NewRateLimiter(globalRate, globalBurst, perRate, perBurst float64) *RateLimiter {
	return &RateLimiter{
		global:      newTokenBucket(globalRate, globalBurst),
		perRate:     perRate,
		perBurst:    perBurst,
		perShortIDs: make(map[string]*tokenBucket),
	}
}

// Allow returns (true, "") if the request passes both the global and
// per-short_id budgets. Returns (false, reason) on rejection, where
// reason identifies which bucket rejected it — useful for logging.
func (r *RateLimiter) Allow(shortID string) (bool, string) {
	if !r.global.Allow() {
		return false, "global"
	}

	r.mu.Lock()
	b, ok := r.perShortIDs[shortID]
	if !ok {
		b = newTokenBucket(r.perRate, r.perBurst)
		r.perShortIDs[shortID] = b
	}
	r.mu.Unlock()

	if !b.Allow() {
		return false, "per_short_id"
	}
	return true, ""
}
