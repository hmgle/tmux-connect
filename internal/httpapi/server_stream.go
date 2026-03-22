package httpapi

import (
	"io"
	"net/http"
	"time"
)

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	pane, ok := requiredPaneQuery(w, r)
	if !ok {
		return
	}
	lines := intQuery(r, "lines", 120)
	stream, err := s.service.OpenStream(r.Context(), pane, lines)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer stream.Subscription.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}

	if err := writeSSE(w, "initial", map[string]any{
		"pane":    stream.Pane.Target.PaneKey(),
		"lines":   lines,
		"content": stream.Initial,
	}); err != nil {
		return
	}
	flusher.Flush()

	heartbeat := time.NewTicker(s.heartbeatInterval)
	defer heartbeat.Stop()
	chunks := stream.Subscription.Chunks()
	errs := stream.Subscription.Errs()

	for {
		if chunks == nil && errs == nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := io.WriteString(w, ":keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err == nil {
				continue
			}
			_ = writeSSE(w, "error", map[string]string{"error": err.Error()})
			flusher.Flush()
			return
		case chunk, ok := <-chunks:
			if !ok {
				chunks = nil
				continue
			}
			if err := writeSSE(w, "output", map[string]any{
				"pane":    stream.Pane.Target.PaneKey(),
				"content": chunk.Text,
				"at":      chunk.ReceivedAt,
			}); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
