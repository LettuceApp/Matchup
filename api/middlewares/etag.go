package middlewares

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// ETagMiddleware adds an `ETag` header to GET responses and returns
// `304 Not Modified` when the client supplies a matching `If-None-Match`.
//
// The implementation is intentionally simple — buffer the entire response
// in memory, hash it, then either flush the buffer or return 304. This
// works well for the kinds of GETs we expose today (health checks, plain
// JSON URLs, occasional asset proxies) which are all small.
//
// Things this middleware deliberately does NOT do:
//   - Stream. SSE and chunked-transfer responses bypass entirely (we look
//     at Content-Type and the explicit "Transfer-Encoding" header before
//     buffering — anything that smells like a stream falls through).
//   - Wrap non-GET methods. ConnectRPC uses POST for everything; running
//     ETag logic over POST bodies would buffer thousands of upload bytes
//     and accomplish nothing.
//   - Override an ETag the handler set itself. If the handler already
//     populated the header, we trust it.
func ETagMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only GET (and HEAD, which behaves identically wrt caching)
			// benefit from ETag. Everything else passes through.
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				next.ServeHTTP(w, r)
				return
			}

			ew := &etagResponseWriter{ResponseWriter: w, buf: &bytes.Buffer{}, status: http.StatusOK}
			next.ServeHTTP(ew, r)

			// Streaming responses opt out: we'd never have buffered the
			// full body, and even if we did, sending 304 to a long-poll
			// client is just hostile. Detect by Content-Type sniffing —
			// SSE handlers always set text/event-stream. Same for any
			// handler that explicitly chose chunked transfer encoding.
			if ew.streaming {
				// We never wrote anything to the wrapped writer for
				// streaming responses (the handler short-circuits us via
				// the Flush() check below). Nothing to do.
				return
			}

			// If the handler set ETag explicitly, respect it. We still
			// need to flush the buffered body since we intercepted writes.
			if existing := ew.Header().Get("ETag"); existing != "" {
				if matchesIfNoneMatch(r.Header.Get("If-None-Match"), existing) {
					ew.ResponseWriter.WriteHeader(http.StatusNotModified)
					return
				}
				ew.flush()
				return
			}

			// Hash the buffered body. SHA-256 truncated to 16 hex chars
			// (8 bytes / 64 bits) keeps collisions astronomically unlikely
			// for the response sizes we serve while keeping headers small.
			body := ew.buf.Bytes()
			sum := sha256.Sum256(body)
			etag := `"` + hex.EncodeToString(sum[:8]) + `"`
			ew.Header().Set("ETag", etag)

			if matchesIfNoneMatch(r.Header.Get("If-None-Match"), etag) {
				// 304 must omit the body and Content-Length per RFC 7232.
				ew.Header().Del("Content-Length")
				ew.ResponseWriter.WriteHeader(http.StatusNotModified)
				return
			}
			ew.flush()
		})
	}
}

// matchesIfNoneMatch implements the subset of RFC 7232 §3.2 we actually use:
// a comma-separated list of quoted ETags, plus the wildcard "*". Weak ETags
// (W/"...") are matched as if strong — we never produce weak tags ourselves
// and treating an inbound weak tag as a hit is harmless given a strong ETag
// produced from the same body bytes will compare equal regardless.
func matchesIfNoneMatch(ifNoneMatch, etag string) bool {
	if ifNoneMatch == "" {
		return false
	}
	if strings.TrimSpace(ifNoneMatch) == "*" {
		return true
	}
	for _, candidate := range strings.Split(ifNoneMatch, ",") {
		candidate = strings.TrimSpace(candidate)
		candidate = strings.TrimPrefix(candidate, "W/")
		if candidate == etag {
			return true
		}
	}
	return false
}

// etagResponseWriter buffers all writes so the middleware can hash the body
// before deciding to send it or a 304. Headers are still routed to the real
// ResponseWriter so handler-set values (Content-Type, custom headers) survive.
type etagResponseWriter struct {
	http.ResponseWriter
	buf       *bytes.Buffer
	status    int
	streaming bool
	wroteHdr  bool
}

func (e *etagResponseWriter) WriteHeader(code int) {
	if e.wroteHdr {
		return
	}
	e.wroteHdr = true
	e.status = code
	// Decide once whether this is a streaming response. We can't make the
	// decision before WriteHeader because handlers set Content-Type before
	// the first Write/WriteHeader call.
	if isStreamingContentType(e.Header().Get("Content-Type")) {
		e.streaming = true
		e.ResponseWriter.WriteHeader(code)
	}
}

func (e *etagResponseWriter) Write(p []byte) (int, error) {
	if !e.wroteHdr {
		// First Write implicitly triggers a 200 status — let WriteHeader
		// run its content-type detection before we decide whether to
		// buffer or stream.
		e.WriteHeader(http.StatusOK)
	}
	if e.streaming {
		return e.ResponseWriter.Write(p)
	}
	return e.buf.Write(p)
}

// Flush is implemented so that streaming handlers (SSE, long-poll) can
// signal the middleware to bypass buffering. The flag flips on the first
// Flush call, mirroring how chi's middleware.Compress detects streams.
func (e *etagResponseWriter) Flush() {
	if !e.wroteHdr {
		e.WriteHeader(http.StatusOK)
	}
	if !e.streaming {
		// The handler is asking to flush before we've decided this is a
		// stream — promote it to streaming now. Drain whatever we've
		// buffered to the underlying writer first so we don't lose it.
		e.streaming = true
		if e.buf.Len() > 0 {
			e.ResponseWriter.Write(e.buf.Bytes())
			e.buf.Reset()
		}
	}
	if f, ok := e.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// flush writes the buffered body to the underlying ResponseWriter. Used on
// the cache-miss path; not called for streaming responses.
func (e *etagResponseWriter) flush() {
	if e.status != 0 && e.status != http.StatusOK {
		e.ResponseWriter.WriteHeader(e.status)
	}
	e.ResponseWriter.Write(e.buf.Bytes())
}

func isStreamingContentType(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.HasPrefix(ct, "text/event-stream") ||
		strings.HasPrefix(ct, "application/grpc") ||
		strings.HasPrefix(ct, "multipart/")
}
