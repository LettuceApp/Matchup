package controllers

import (
	"fmt"
	"net/http"
	"time"

	"Matchup/cache"

	"github.com/go-chi/chi/v5"
)

// BracketEventsSSE streams bracket advance events to the client via Server-Sent Events.
// Clients connect to GET /brackets/{bracketID}/events and receive a "data: advance\n\n"
// message whenever the bracket round advances. A keepalive comment is sent every 30s
// to prevent proxy timeouts.
func (h *BracketHandler) BracketEventsSSE(w http.ResponseWriter, r *http.Request) {
	bracketPublicID := chi.URLParam(r, "bracketID")

	bracketRecord, err := resolveBracketByIdentifier(h.DB, bracketPublicID)
	if err != nil {
		http.Error(w, "bracket not found", http.StatusNotFound)
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

	// Degrade gracefully when Redis is unavailable — hold connection open until client disconnects.
	if cache.Client == nil {
		<-r.Context().Done()
		return
	}

	channel := fmt.Sprintf("bracket:%d:events", bracketRecord.ID)
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
			// SSE keepalive comment — prevents proxy timeouts
			fmt.Fprintf(w, ":\n\n")
			flusher.Flush()

		case <-r.Context().Done():
			return
		}
	}
}


