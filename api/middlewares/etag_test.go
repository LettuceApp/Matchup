package middlewares

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// helloHandler always responds 200 with the same body — useful for ETag
// stability tests where we want repeated calls to produce the same hash.
func helloHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"hello":"world"}`))
	}
}

func TestETag_GeneratedAndStable(t *testing.T) {
	handler := ETagMiddleware()(helloHandler())

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	first := w.Header().Get("ETag")
	if first == "" || !strings.HasPrefix(first, `"`) {
		t.Fatalf("expected quoted ETag, got %q", first)
	}

	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/x", nil))
	if got := w2.Header().Get("ETag"); got != first {
		t.Fatalf("ETag should be stable across calls — first=%q second=%q", first, got)
	}
}

func TestETag_NotModifiedOnIfNoneMatch(t *testing.T) {
	handler := ETagMiddleware()(helloHandler())

	// First request to learn the ETag.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag on first response")
	}

	// Second request with If-None-Match should yield 304 and empty body.
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("If-None-Match", etag)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req)

	if w2.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", w2.Code)
	}
	if w2.Body.Len() != 0 {
		t.Fatalf("304 response must have empty body, got %d bytes", w2.Body.Len())
	}
}

func TestETag_PostMethodPassesThrough(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"ok":true}`))
	})
	handler := ETagMiddleware()(inner)

	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Fatal("inner handler was not invoked for POST")
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	if w.Header().Get("ETag") != "" {
		t.Fatal("ETag must not be set on POST responses")
	}
}

func TestETag_StreamingContentTypeBypasses(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: hello\n\n"))
	})
	handler := ETagMiddleware()(inner)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/sse", nil))

	if w.Header().Get("ETag") != "" {
		t.Fatal("streaming response must not have an ETag header")
	}
	if !strings.Contains(w.Body.String(), "data: hello") {
		t.Fatalf("streaming body lost: %q", w.Body.String())
	}
}

func TestMatchesIfNoneMatch(t *testing.T) {
	cases := []struct {
		header string
		etag   string
		want   bool
	}{
		{`"abc"`, `"abc"`, true},
		{`"abc", "def"`, `"def"`, true},
		{`*`, `"anything"`, true},
		{``, `"abc"`, false},
		{`"xyz"`, `"abc"`, false},
		{`W/"abc"`, `"abc"`, true}, // weak tag treated as strong for our purposes
	}
	for _, c := range cases {
		if got := matchesIfNoneMatch(c.header, c.etag); got != c.want {
			t.Errorf("matchesIfNoneMatch(%q, %q) = %v, want %v", c.header, c.etag, got, c.want)
		}
	}
}
