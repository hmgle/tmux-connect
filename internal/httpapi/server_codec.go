package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

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
	switch tmuxconn.ExitCode(err) {
	case tmuxconn.ExitUsage:
		status = http.StatusBadRequest
	case tmuxconn.ExitNotFound:
		status = http.StatusNotFound
	case tmuxconn.ExitTmuxFailure:
		status = http.StatusBadGateway
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
