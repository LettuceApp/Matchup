package middlewares

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus metrics for HTTP traffic.
//
// `http_requests_total` — one increment per handled request, labelled by
// method, route pattern, and status code. Route pattern (not raw path) is
// used to keep cardinality bounded: 1,000 users browsing their profiles hit
// the same `/users/{id}` pattern, not 1,000 separate series.
//
// `http_request_duration_seconds` — histogram of request latencies, bucketed
// to cover fast API calls (<10ms) up through slow uploads (~10s). The
// default buckets would miss our p99s on image uploads.
var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests handled, partitioned by method, route, and status.",
	}, []string{"method", "route", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "Request latency in seconds, partitioned by method and route.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"method", "route"})
)

// statusRecorder wraps http.ResponseWriter so the middleware can read back
// the status the handler wrote. Defaults to 200 (the net/http default when
// nothing is written explicitly).
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// MetricsMiddleware records request counts and durations for every handled
// request. Must be mounted AFTER chi has routed the request so
// chi.RouteContext(ctx).RoutePattern() returns the matched template (e.g.
// "/users/{id}") rather than the literal URL. In practice that means placing
// it near the top of the middleware chain — chi populates the route context
// during ServeHTTP regardless of ordering, and the pattern is read after
// next.ServeHTTP.
func MetricsMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip the metrics endpoint itself — scraping our own scraper
			// creates a feedback loop that's useless (every scrape bumps
			// the counter for the scrape) and inflates the /metrics
			// latency histogram with its own cost.
			if r.URL.Path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}

			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(rec, r)
			elapsed := time.Since(start).Seconds()

			// Fall back to the raw path if chi hasn't populated a pattern
			// (e.g. 404s for unmatched routes). We still cap cardinality
			// by bucketing unknown paths as "unmatched".
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = "unmatched"
			}

			labels := prometheus.Labels{
				"method": r.Method,
				"route":  route,
				"status": strconv.Itoa(rec.status),
			}
			httpRequestsTotal.With(labels).Inc()
			httpRequestDuration.With(prometheus.Labels{
				"method": r.Method,
				"route":  route,
			}).Observe(elapsed)
		})
	}
}
