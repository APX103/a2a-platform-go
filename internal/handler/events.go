package handler

import (
	"fmt"
	"net/http"
	"time"

	"a2a-platform/internal/events"
)

// EventsHandler serves the SSE stream at /api/events.
type EventsHandler struct {
	broadcaster *events.Broadcaster
}

func NewEventsHandler(b *events.Broadcaster) *EventsHandler {
	return &EventsHandler{broadcaster: b}
}

func (h *EventsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", 405)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonError(w, "streaming not supported", 500)
		return
	}

	sessionID := fmt.Sprintf("%s-%d", r.RemoteAddr, time.Now().UnixNano())
	ch := h.broadcaster.Subscribe(sessionID)
	defer h.broadcaster.Unsubscribe(sessionID)

	// Send initial connection event
	fmt.Fprintf(w, ":ok\n\n")
	fmt.Fprintf(w, "event: connected\n")
	fmt.Fprintf(w, "data: %s\n\n", `{"status":"connected"}`)
	flusher.Flush()

	// Keepalive ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			w.Write([]byte(msg))
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
