package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/hmgle/tmux-connect/internal/tagb"
)

type BridgeService interface {
	List(ctx context.Context) ([]tagb.PaneRecord, error)
	Attach(ctx context.Context, ref string, agent string, label string) (tagb.PaneRecord, error)
	Detach(ctx context.Context, ref string) error
	Inspect(ctx context.Context, ref string) (tagb.PaneRecord, error)
	Snapshot(ctx context.Context, ref string, lines int) (string, error)
	Send(ctx context.Context, ref string, text string, sendEnter bool) error
	Enter(ctx context.Context, ref string) error
	CtrlC(ctx context.Context, ref string) error
	OpenStream(ctx context.Context, ref string, lines int) (tagb.PaneStream, error)
}

type Server struct {
	addr              string
	service           BridgeService
	server            *http.Server
	heartbeatInterval time.Duration
}

func NewServer(addr string, service BridgeService) *Server {
	s := &Server{
		addr:              addr,
		service:           service,
		heartbeatInterval: 20 * time.Second,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /v1/panes", s.handleList)
	mux.HandleFunc("POST /v1/panes/attach", s.handleAttach)
	mux.HandleFunc("POST /v1/panes/detach", s.handleDetach)
	mux.HandleFunc("GET /v1/panes/inspect", s.handleInspect)
	mux.HandleFunc("GET /v1/panes/snapshot", s.handleSnapshot)
	mux.HandleFunc("POST /v1/panes/send", s.handleSend)
	mux.HandleFunc("POST /v1/panes/enter", s.handleEnter)
	mux.HandleFunc("POST /v1/panes/ctrl-c", s.handleCtrlC)
	mux.HandleFunc("GET /v1/panes/stream", s.handleStream)
	s.server = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

func (s *Server) Addr() string {
	return s.addr
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		errCh <- s.server.Shutdown(shutdownCtx)
	}()

	err := s.server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		shutdownErr := <-errCh
		if shutdownErr != nil && !errors.Is(shutdownErr, context.Canceled) {
			return shutdownErr
		}
		return nil
	}
	return err
}

type attachRequest struct {
	Pane  string `json:"pane"`
	Agent string `json:"agent"`
	Label string `json:"label"`
}

type paneRequest struct {
	Pane string `json:"pane"`
}

type sendRequest struct {
	Pane  string `json:"pane"`
	Text  string `json:"text"`
	Enter bool   `json:"enter"`
}

type snapshotResponse struct {
	Pane     string `json:"pane"`
	Lines    int    `json:"lines"`
	Snapshot string `json:"snapshot"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"time": time.Now(),
	})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	records, err := s.service.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"panes": records})
}

func (s *Server) handleAttach(w http.ResponseWriter, r *http.Request) {
	var req attachRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	record, err := s.service.Attach(r.Context(), req.Pane, req.Agent, req.Label)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) handleDetach(w http.ResponseWriter, r *http.Request) {
	var req paneRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.service.Detach(r.Context(), req.Pane); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"detached": true, "pane": req.Pane})
}

func (s *Server) handleInspect(w http.ResponseWriter, r *http.Request) {
	pane, ok := requiredPaneQuery(w, r)
	if !ok {
		return
	}
	record, err := s.service.Inspect(r.Context(), pane)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	pane, ok := requiredPaneQuery(w, r)
	if !ok {
		return
	}
	lines := intQuery(r, "lines", 120)
	body, err := s.service.Snapshot(r.Context(), pane, lines)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshotResponse{Pane: pane, Lines: lines, Snapshot: body})
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	var req sendRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.service.Send(r.Context(), req.Pane, req.Text, req.Enter); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": true, "pane": req.Pane, "enter": req.Enter})
}

func (s *Server) handleEnter(w http.ResponseWriter, r *http.Request) {
	var req paneRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.service.Enter(r.Context(), req.Pane); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": true, "pane": req.Pane, "key": "Enter"})
}

func (s *Server) handleCtrlC(w http.ResponseWriter, r *http.Request) {
	var req paneRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.service.CtrlC(r.Context(), req.Pane); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": true, "pane": req.Pane, "key": "C-c"})
}

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

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("request body must contain a single JSON object")
		}
		return err
	}
	return nil
}

func requiredPaneQuery(w http.ResponseWriter, r *http.Request) (string, bool) {
	pane := r.URL.Query().Get("pane")
	if pane == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing pane query parameter"})
		return "", false
	}
	return pane, true
}

func intQuery(r *http.Request, key string, defaultValue int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultValue
	}
	return value
}

func writeSSE(w http.ResponseWriter, event string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", body); err != nil {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeServiceError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch tagb.ExitCode(err) {
	case tagb.ExitUsage:
		status = http.StatusBadRequest
	case tagb.ExitNotFound:
		status = http.StatusNotFound
	case tagb.ExitTmuxFailure:
		status = http.StatusBadGateway
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
