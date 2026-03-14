package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/portgle/tmux-connect/internal/tagb"
	"github.com/portgle/tmux-connect/internal/tmux"
)

type stubService struct {
	listFn       func(context.Context) ([]tagb.PaneRecord, error)
	attachFn     func(context.Context, string, string, string) (tagb.PaneRecord, error)
	detachFn     func(context.Context, string) error
	inspectFn    func(context.Context, string) (tagb.PaneRecord, error)
	snapshotFn   func(context.Context, string, int) (string, error)
	sendFn       func(context.Context, string, string, bool) error
	enterFn      func(context.Context, string) error
	ctrlCFn      func(context.Context, string) error
	openStreamFn func(context.Context, string, int) (tagb.PaneStream, error)
}

func (s stubService) List(ctx context.Context) ([]tagb.PaneRecord, error) {
	return s.listFn(ctx)
}
func (s stubService) Attach(ctx context.Context, ref, agent, label string) (tagb.PaneRecord, error) {
	return s.attachFn(ctx, ref, agent, label)
}
func (s stubService) Detach(ctx context.Context, ref string) error { return s.detachFn(ctx, ref) }
func (s stubService) Inspect(ctx context.Context, ref string) (tagb.PaneRecord, error) {
	return s.inspectFn(ctx, ref)
}
func (s stubService) Snapshot(ctx context.Context, ref string, lines int) (string, error) {
	return s.snapshotFn(ctx, ref, lines)
}
func (s stubService) Send(ctx context.Context, ref, text string, sendEnter bool) error {
	return s.sendFn(ctx, ref, text, sendEnter)
}
func (s stubService) Enter(ctx context.Context, ref string) error { return s.enterFn(ctx, ref) }
func (s stubService) CtrlC(ctx context.Context, ref string) error { return s.ctrlCFn(ctx, ref) }
func (s stubService) OpenStream(ctx context.Context, ref string, lines int) (tagb.PaneStream, error) {
	return s.openStreamFn(ctx, ref, lines)
}

func TestHandleList(t *testing.T) {
	t.Parallel()

	srv := NewServer("127.0.0.1:0", stubService{
		listFn: func(context.Context) ([]tagb.PaneRecord, error) {
			return []tagb.PaneRecord{{
				Info: tmux.PaneInfo{
					Target:      tmux.Target{Socket: "default", PaneID: "%1"},
					SessionName: "dev",
					WindowName:  "shell",
					CurrentCmd:  "zsh",
				},
				Metadata: tmux.BridgeMetadata{Managed: true, Mode: tmux.ModeRelay, Agent: tmux.AgentClaude},
			}}, nil
		},
		attachFn:     func(context.Context, string, string, string) (tagb.PaneRecord, error) { return tagb.PaneRecord{}, nil },
		detachFn:     func(context.Context, string) error { return nil },
		inspectFn:    func(context.Context, string) (tagb.PaneRecord, error) { return tagb.PaneRecord{}, nil },
		snapshotFn:   func(context.Context, string, int) (string, error) { return "", nil },
		sendFn:       func(context.Context, string, string, bool) error { return nil },
		enterFn:      func(context.Context, string) error { return nil },
		ctrlCFn:      func(context.Context, string) error { return nil },
		openStreamFn: func(context.Context, string, int) (tagb.PaneStream, error) { return tagb.PaneStream{}, nil },
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/panes", nil)
	rec := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q", got)
	}
}

func TestHandleSend(t *testing.T) {
	t.Parallel()

	var gotPane, gotText string
	var gotEnter bool
	srv := NewServer("127.0.0.1:0", stubService{
		listFn:     func(context.Context) ([]tagb.PaneRecord, error) { return nil, nil },
		attachFn:   func(context.Context, string, string, string) (tagb.PaneRecord, error) { return tagb.PaneRecord{}, nil },
		detachFn:   func(context.Context, string) error { return nil },
		inspectFn:  func(context.Context, string) (tagb.PaneRecord, error) { return tagb.PaneRecord{}, nil },
		snapshotFn: func(context.Context, string, int) (string, error) { return "", nil },
		sendFn: func(_ context.Context, ref, text string, sendEnter bool) error {
			gotPane, gotText, gotEnter = ref, text, sendEnter
			return nil
		},
		enterFn:      func(context.Context, string) error { return nil },
		ctrlCFn:      func(context.Context, string) error { return nil },
		openStreamFn: func(context.Context, string, int) (tagb.PaneStream, error) { return tagb.PaneStream{}, nil },
	})

	body := bytes.NewBufferString(`{"pane":"%5","text":"hello","enter":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/panes/send", body)
	rec := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if gotPane != "%5" || gotText != "hello" || !gotEnter {
		t.Fatalf("unexpected payload: pane=%q text=%q enter=%t", gotPane, gotText, gotEnter)
	}
}

func TestHandleStream(t *testing.T) {
	t.Parallel()

	sub := tmux.NewSubscriptionForTest()
	sub.PushChunk(tmux.OutputChunk{
		Target:     tmux.Target{Socket: "default", PaneID: "%5"},
		Text:       "next line\n",
		ReceivedAt: time.Unix(100, 0),
	})
	sub.CloseChannels()

	srv := NewServer("127.0.0.1:0", stubService{
		listFn:     func(context.Context) ([]tagb.PaneRecord, error) { return nil, nil },
		attachFn:   func(context.Context, string, string, string) (tagb.PaneRecord, error) { return tagb.PaneRecord{}, nil },
		detachFn:   func(context.Context, string) error { return nil },
		inspectFn:  func(context.Context, string) (tagb.PaneRecord, error) { return tagb.PaneRecord{}, nil },
		snapshotFn: func(context.Context, string, int) (string, error) { return "", nil },
		sendFn:     func(context.Context, string, string, bool) error { return nil },
		enterFn:    func(context.Context, string) error { return nil },
		ctrlCFn:    func(context.Context, string) error { return nil },
		openStreamFn: func(context.Context, string, int) (tagb.PaneStream, error) {
			return tagb.PaneStream{
				Pane:         tmux.PaneInfo{Target: tmux.Target{Socket: "default", PaneID: "%5"}},
				Initial:      "hello\n",
				Subscription: sub,
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/panes/stream?pane=%255", nil)
	rec := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body == "" || !bytes.Contains(rec.Body.Bytes(), []byte("event: initial")) || !bytes.Contains(rec.Body.Bytes(), []byte("event: output")) {
		t.Fatalf("unexpected SSE body: %q", body)
	}
}

func TestWriteServiceError(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	writeServiceError(rec, tagb.NotFoundError("pane not found"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["error"] == "" {
		t.Fatal("expected error message")
	}
}
