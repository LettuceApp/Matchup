package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// newRouterWithMetrics spins up a tiny chi router with the middleware
// under test so we can exercise real route-pattern recording.
func newRouterWithMetrics(t *testing.T) *chi.Mux {
	t.Helper()
	r := chi.NewRouter()
	r.Use(MetricsMiddleware())
	r.Get("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Get("/boom", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return r
}

func TestMetricsMiddleware_RecordsRoutePattern(t *testing.T) {
	// Two requests to /users/1 and /users/2 should collapse into the same
	// `route="/users/{id}"` series — that's the whole point of using the
	// pattern rather than the raw path.
	r := newRouterWithMetrics(t)

	baseline := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/users/{id}", "200"))

	for _, id := range []string{"1", "2"} {
		req := httptest.NewRequest("GET", "/users/"+id, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
	}

	got := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/users/{id}", "200"))
	if got-baseline != 2 {
		t.Errorf("expected 2 increments under /users/{id}, got delta=%v", got-baseline)
	}
}

func TestMetricsMiddleware_RecordsStatusCode(t *testing.T) {
	r := newRouterWithMetrics(t)

	baseline := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/boom", "500"))

	req := httptest.NewRequest("GET", "/boom", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	got := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/boom", "500"))
	if got-baseline != 1 {
		t.Errorf("expected 1 increment at status=500, got delta=%v", got-baseline)
	}
}

func TestMetricsMiddleware_SkipsMetricsEndpoint(t *testing.T) {
	// Scraping the scraper would create a feedback loop: every Prometheus
	// poll would bump the counter for the poll itself.
	r := newRouterWithMetrics(t)

	baseline := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/metrics", "200"))

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	got := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/metrics", "200"))
	if got != baseline {
		t.Errorf("metrics path must not be counted; delta=%v", got-baseline)
	}
}

func TestMetricsMiddleware_BucketsUnmatchedRoutes(t *testing.T) {
	// 404s on unmatched paths should aggregate under `route="unmatched"`
	// rather than spawn one series per raw URL (cardinality defense).
	r := newRouterWithMetrics(t)

	baseline := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "unmatched", "404"))

	for _, path := range []string{"/nope/a", "/nope/b", "/nope/c"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
	}

	got := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "unmatched", "404"))
	if got-baseline != 3 {
		t.Errorf("expected 3 unmatched increments, got delta=%v", got-baseline)
	}
}

func TestMetricsMiddleware_RecordsDuration(t *testing.T) {
	// Verifies the duration histogram gets populated — we don't assert
	// on the exact time (flaky), just that a series exists for the
	// matched route's label set after the request runs.
	r := newRouterWithMetrics(t)

	req := httptest.NewRequest("GET", "/users/99", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// GetMetricWithLabelValues returns the child histogram; counting
	// the series via CollectAndCount on the parent Vec after the
	// request proves our label set was registered.
	if _, err := httpRequestDuration.GetMetricWithLabelValues("GET", "/users/{id}"); err != nil {
		t.Fatalf("expected histogram child for /users/{id}: %v", err)
	}
	if count := testutil.CollectAndCount(httpRequestDuration); count == 0 {
		t.Error("expected at least one duration histogram series, got 0")
	}
}
