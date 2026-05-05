package controllers

import (
	"fmt"
	"net/http"
	"time"

	"Matchup/cache"
	httpctx "Matchup/utils/httpctx"
)

// ActivityEventsSSE streams per-user activity-feed change notifications
// via Server-Sent Events. Copies the shape of BracketEventsSSE, but the
// subscribe channel is keyed to the authenticated user rather than a
// URL parameter — `GET /users/me/activity/events` is the only route.
//
// Payload is intentionally just "ping": clients refetch the full feed
// on any event. Matches the derived-feed read model where the list is
// computed fresh per request.
//
// Auth: requires a logged-in user. Anonymous callers receive 401.
// Degrades gracefully when Redis is unavailable — holds the connection
// open until the client disconnects instead of 500-ing.
func (h *ActivityHandler) ActivityEventsSSE(w http.ResponseWriter, r *http.Request) {
	uid, ok := httpctx.CurrentUserID(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Graceful degrade when Redis is down — keep the connection open
	// so the client's EventSource doesn't reconnect-storm. The client
	// falls back to polling either way.
	if cache.Client == nil {
		<-r.Context().Done()
		return
	}

	channel := cache.ActivityChannel(uid)
	pubsub := cache.Client.Subscribe(r.Context(), channel)
	defer pubsub.Close()

	msgCh := pubsub.Channel()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-msgCh:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg.Payload)
			flusher.Flush()

		case <-ticker.C:
			// SSE keepalive — prevents proxy / LB idle timeouts.
			fmt.Fprint(w, ":\n\n")
			flusher.Flush()

		case <-r.Context().Done():
			return
		}
	}
}
