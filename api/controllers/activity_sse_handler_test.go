package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"Matchup/cache"
	httpctx "Matchup/utils/httpctx"
)

// These tests cover the SSE handler's auth gate, SSE-header contract,
// and the Redis-nil graceful-degrade path. The live subscribe→publish
// round-trip isn't tested here — the rest of the project doesn't use
// a real Redis in unit tests (see middlewares/rate_limit_test.go which
// only covers the nil path too), and curl / browser smoke tests cover
// that shape. What we can and should test is:
//
//   1. Anonymous callers get 401 (no SSE headers written).
//   2. Authed callers get the SSE headers and the handler stays open
//      until the request context is cancelled.
//   3. cache.Client == nil → handler holds the connection until ctx
//      closes rather than 500-ing or busy-looping.

// newTestSSERequest builds a GET to the SSE endpoint with an optional
// authenticated uid baked into its context. Returns the request + a
// cancel func the test can use to simulate the client disconnecting.
func newTestSSERequest(t *testing.T, uid uint) (*http.Request, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	if uid != 0 {
		ctx = httpctx.WithUserID(ctx, uid)
	}
	req := httptest.NewRequest(http.MethodGet, "/users/me/activity/events", nil).WithContext(ctx)
	return req, cancel
}

func TestActivityEventsSSE_RejectsUnauthenticated(t *testing.T) {
	h := &ActivityHandler{}
	req, cancel := newTestSSERequest(t, 0)
	defer cancel()
	w := httptest.NewRecorder()

	h.ActivityEventsSSE(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for anon caller, got %d", w.Code)
	}
	// Should NOT have started SSE streaming headers — if they were set,
	// a naive EventSource client would stay connected thinking the
	// stream is live.
	if got := w.Header().Get("Content-Type"); got == "text/event-stream" {
		t.Errorf("expected no SSE Content-Type on 401; got %q", got)
	}
}

func TestActivityEventsSSE_RedisNilHoldsUntilCtxDone(t *testing.T) {
	prev := cache.Client
	cache.Client = nil
	t.Cleanup(func() { cache.Client = prev })

	h := &ActivityHandler{}
	req, cancel := newTestSSERequest(t, 42)
	w := httptest.NewRecorder()

	// Run the handler; cancel the request context shortly after so the
	// degrade branch returns. If the handler accidentally 500s or
	// busy-loops, the goroutine won't close and the test times out.
	done := make(chan struct{})
	go func() {
		h.ActivityEventsSSE(w, req)
		close(done)
	}()

	time.Sleep(25 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ActivityEventsSSE did not return after ctx cancel with cache.Client == nil")
	}

	// SSE contract holds even on the degrade path: Content-Type must be
	// text/event-stream so EventSource clients don't choke. Status stays
	// 200 (Go's default once the body is written to).
	if got := w.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %q", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("expected Cache-Control no-cache, got %q", got)
	}
	if got := w.Header().Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("expected X-Accel-Buffering no, got %q", got)
	}
	// Body should be empty — no pings, no keepalives, nothing. Clients
	// fall back to the 60s poll loop.
	if body := w.Body.String(); body != "" {
		t.Errorf("expected empty body in degrade path, got %q", body)
	}
}

// TestActivityEventsSSE_NoFlusher documents what happens when the
// underlying ResponseWriter can't stream — currently we 500. The
// test uses a bare ResponseWriter that does NOT implement http.Flusher.
// httptest.ResponseRecorder DOES implement Flush, so we wrap it.
func TestActivityEventsSSE_NoFlusher(t *testing.T) {
	h := &ActivityHandler{}
	req, cancel := newTestSSERequest(t, 42)
	defer cancel()

	// Wrap a ResponseRecorder in a struct that intentionally does NOT
	// forward the Flush method. Keeps Header() / Write() / WriteHeader().
	rec := httptest.NewRecorder()
	w := noFlushWriter{ResponseWriter: rec}

	h.ActivityEventsSSE(w, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when flusher unavailable, got %d", rec.Code)
	}
}

// noFlushWriter deliberately shadows http.Flusher so type assertions
// fail. Used only by TestActivityEventsSSE_NoFlusher.
type noFlushWriter struct {
	http.ResponseWriter
}
