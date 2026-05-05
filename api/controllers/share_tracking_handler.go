package controllers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// shareLandedTotal counts arrivals from a shared link. Labels are
// bounded: `source` is validated against a fixed allow-list (see
// allowedShareSources) so nobody can inflate cardinality by POSTing
// arbitrary strings. `content_type` is matchup / bracket / other.
var shareLandedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "share_landed_total",
	Help: "Total page landings from a shared link, labelled by source channel and content type.",
}, []string{"source", "content_type"})

// Allow-list for the `source` label. Anything outside this set gets
// bucketed as "other" — keeps cardinality locked regardless of what
// the frontend (or an attacker) sends.
var allowedShareSources = map[string]struct{}{
	"tw":      {},
	"fb":      {},
	"li":      {},
	"copy":    {},
	"native":  {},
	"email":   {},
	"sms":     {},
	"ig":      {},
	"dm":      {},
	"discord": {},
	"slack":   {},
}

// Allow-list for content_type. The frontend currently only emits
// matchup or bracket; "other" catches future kinds before we extend
// the allow-list.
var allowedShareContentTypes = map[string]struct{}{
	"matchup": {},
	"bracket": {},
}

// shareLandedRequest mirrors the frontend sendBeacon body.
type shareLandedRequest struct {
	Source      string `json:"source"`
	ContentType string `json:"content_type"`
	ShortID     string `json:"short_id"` // currently unused server-side, accepted for forward compat
}

// ShareLandedHandler accepts the attribution beacon from the frontend
// when a user lands on a page via a `?ref=<channel>` URL. Does not
// authenticate — this is a public counter, and anything more involved
// would just encourage frontend to abandon sendBeacon's fire-and-forget
// semantics.
//
// Failure modes are silent (log-and-200) rather than error responses
// because sendBeacon ignores the status anyway; a 4xx would just show
// up as noise in the browser console.
func ShareLandedHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var body shareLandedRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log.Printf("share_landed: decode: %v", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	source := body.Source
	if _, ok := allowedShareSources[source]; !ok {
		source = "other"
	}
	contentType := body.ContentType
	if _, ok := allowedShareContentTypes[contentType]; !ok {
		contentType = "other"
	}

	shareLandedTotal.WithLabelValues(source, contentType).Inc()
	w.WriteHeader(http.StatusNoContent)
}
