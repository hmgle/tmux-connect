package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

type BridgeService interface {
	List(ctx context.Context) ([]tmuxconn.PaneRecord, error)
	Attach(ctx context.Context, ref string, agent string, label string) (tmuxconn.PaneRecord, error)
	Detach(ctx context.Context, ref string) error
	Inspect(ctx context.Context, ref string) (tmuxconn.PaneRecord, error)
	Snapshot(ctx context.Context, ref string, lines int) (string, error)
	Send(ctx context.Context, ref string, text string, sendEnter bool) error
	Enter(ctx context.Context, ref string) error
	CtrlC(ctx context.Context, ref string) error
	OpenStream(ctx context.Context, ref string, lines int) (tmuxconn.PaneStream, error)
}

type Server struct {
	addr              string
	service           BridgeService
	server            *http.Server
	heartbeatInterval time.Duration
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

func NewServer(addr string, service BridgeService) *Server {
	s := &Server{
		addr:              addr,
		service:           service,
		heartbeatInterval: 20 * time.Second,
	}
	s.server = &http.Server{
		Addr:              addr,
		Handler:           s.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

func (s *Server) routes() http.Handler {
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
	return mux
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
