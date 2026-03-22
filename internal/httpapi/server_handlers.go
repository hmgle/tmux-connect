package httpapi

import (
	"net/http"
	"time"
)

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
