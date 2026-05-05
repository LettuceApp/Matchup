package share

import (
	"testing"
	"time"
)

func TestTokenBucket_AllowsUpToBurstThenRefills(t *testing.T) {
	// Burst 5, 10 rps — first 5 requests pass, next one denied, then
	// after 100ms (one token's worth of refill) one more passes.
	b := newTokenBucket(10, 5)

	for i := 0; i < 5; i++ {
		if !b.Allow() {
			t.Fatalf("expected burst request %d/5 to pass", i+1)
		}
	}
	if b.Allow() {
		t.Error("6th request in burst should be denied")
	}

	// Fake clock movement: fast-forward the bucket's `last` timestamp.
	b.mu.Lock()
	b.last = b.last.Add(-120 * time.Millisecond) // 1.2 tokens of headroom
	b.mu.Unlock()
	if !b.Allow() {
		t.Error("expected one token available after 120ms refill")
	}
}

func TestRateLimiter_PerShortIDIsolation(t *testing.T) {
	// Exhausting one short_id's budget must not impact another's —
	// the whole point of the per-bucket split is that one noisy URL
	// can't starve a different one.
	rl := NewRateLimiter(1000, 1000, 10, 2) // generous global; tight per

	// Burn through short "A"'s per-bucket burst (2).
	for i := 0; i < 2; i++ {
		ok, _ := rl.Allow("A")
		if !ok {
			t.Fatalf("unexpected deny on A burst %d", i)
		}
	}
	if ok, _ := rl.Allow("A"); ok {
		t.Error("3rd request on A should be denied by per-short_id bucket")
	}
	// B should still have its full burst.
	if ok, _ := rl.Allow("B"); !ok {
		t.Error("B's first request should not be blocked by A's exhaustion")
	}
}

func TestRateLimiter_GlobalRejectReason(t *testing.T) {
	// When the global bucket is empty but the per-short_id bucket has
	// room, we should still reject — and the reason string should
	// identify "global" so log analysis can attribute throttling
	// correctly.
	rl := NewRateLimiter(0.001, 1, 100, 100) // global almost empty, per generous
	if ok, _ := rl.Allow("X"); !ok {
		t.Fatal("first request should pass (global burst=1)")
	}
	ok, reason := rl.Allow("X")
	if ok {
		t.Fatal("second request should hit the empty global bucket")
	}
	if reason != "global" {
		t.Errorf("expected reason=global, got %q", reason)
	}
}
