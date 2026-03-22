package tmuxconn

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hmgle/tmux-connect/internal/buildinfo"
	"github.com/hmgle/tmux-connect/internal/tmux"
)

func TestAppRunVersionText(t *testing.T) {
	restore := setBuildInfoForTest(t, "v0.1.0", "abc1234", "2026-03-22T12:00:00Z")
	defer restore()

	app, stdout, stderr := newTestApp(t, nil)

	if err := app.Run(context.Background(), []string{"version"}); err != nil {
		t.Fatalf("Run(version) error = %v", err)
	}
	if got := stdout.String(); got != "tmux-connect v0.1.0\ncommit: abc1234\nbuilt: 2026-03-22T12:00:00Z\n" {
		t.Fatalf("stdout = %q, want formatted version output", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestAppRunVersionJSON(t *testing.T) {
	restore := setBuildInfoForTest(t, "v0.1.0", "abc1234", "2026-03-22T12:00:00Z")
	defer restore()

	app, stdout, _ := newTestApp(t, nil)

	if err := app.Run(context.Background(), []string{"version", "--json"}); err != nil {
		t.Fatalf("Run(version --json) error = %v", err)
	}

	var payload struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
		Date    string `json:"date"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v\nstdout=%s", err, stdout.String())
	}
	if payload.Version != "v0.1.0" || payload.Commit != "abc1234" || payload.Date != "2026-03-22T12:00:00Z" {
		t.Fatalf("payload = %#v, want version metadata", payload)
	}
}

func TestAppRunListJSON(t *testing.T) {
	t.Parallel()

	app, stdout, _ := newTestApp(t, func(_ context.Context, _ []byte, args ...string) (tmux.RunResult, error) {
		switch args[0] {
		case "list-panes":
			return tmux.RunResult{Stdout: paneStateRow()}, nil
		default:
			t.Fatalf("unexpected args: %v", args)
			return tmux.RunResult{}, nil
		}
	})

	if err := app.Run(context.Background(), []string{"list", "--json"}); err != nil {
		t.Fatalf("Run(list --json) error = %v", err)
	}

	var records []PaneRecord
	if err := json.Unmarshal(stdout.Bytes(), &records); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v\nstdout=%s", err, stdout.String())
	}
	if len(records) != 1 {
		t.Fatalf("records = %#v, want one record", records)
	}
	if records[0].Info.Target.PaneKey() != "default:%5" {
		t.Fatalf("pane = %q, want default:%%5", records[0].Info.Target.PaneKey())
	}
	if !records[0].Metadata.Managed || records[0].Metadata.Agent != tmux.AgentCodex {
		t.Fatalf("metadata = %#v, want managed codex record", records[0].Metadata)
	}
}

func TestAppRunAttachJSON(t *testing.T) {
	t.Parallel()

	app, stdout, _ := newTestApp(t, func(_ context.Context, _ []byte, args ...string) (tmux.RunResult, error) {
		switch args[0] {
		case "list-panes":
			return tmux.RunResult{Stdout: paneInfoRow()}, nil
		case "set-option":
			return tmux.RunResult{}, nil
		default:
			t.Fatalf("unexpected args: %v", args)
			return tmux.RunResult{}, nil
		}
	})

	if err := app.Run(context.Background(), []string{"attach", "--pane", "%5", "--agent", "codex", "--label", "api", "--json"}); err != nil {
		t.Fatalf("Run(attach --json) error = %v", err)
	}

	var record PaneRecord
	if err := json.Unmarshal(stdout.Bytes(), &record); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v\nstdout=%s", err, stdout.String())
	}
	if record.Info.Target.PaneKey() != "default:%5" {
		t.Fatalf("pane = %q, want default:%%5", record.Info.Target.PaneKey())
	}
	if !record.Metadata.Managed || record.Metadata.Agent != tmux.AgentCodex || record.Metadata.Label != "api" {
		t.Fatalf("metadata = %#v, want attached codex/api record", record.Metadata)
	}
}

func TestAppRunDetachJSON(t *testing.T) {
	t.Parallel()

	app, stdout, _ := newTestApp(t, func(_ context.Context, _ []byte, args ...string) (tmux.RunResult, error) {
		switch args[0] {
		case "list-panes", "set-option":
			if args[0] == "list-panes" {
				return tmux.RunResult{Stdout: paneInfoRow()}, nil
			}
			return tmux.RunResult{}, nil
		default:
			t.Fatalf("unexpected args: %v", args)
			return tmux.RunResult{}, nil
		}
	})

	if err := app.Run(context.Background(), []string{"detach", "--pane", "%5", "--json"}); err != nil {
		t.Fatalf("Run(detach --json) error = %v", err)
	}

	var payload struct {
		Pane     string `json:"pane"`
		Detached bool   `json:"detached"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v\nstdout=%s", err, stdout.String())
	}
	if payload.Pane != "%5" || !payload.Detached {
		t.Fatalf("payload = %#v, want detached %%5", payload)
	}
}

func TestAppRunInspectText(t *testing.T) {
	t.Parallel()

	app, stdout, _ := newTestApp(t, func(_ context.Context, _ []byte, args ...string) (tmux.RunResult, error) {
		switch args[0] {
		case "list-panes":
			return tmux.RunResult{Stdout: paneStateRow()}, nil
		default:
			t.Fatalf("unexpected args: %v", args)
			return tmux.RunResult{}, nil
		}
	})

	if err := app.Run(context.Background(), []string{"inspect", "--pane", "%5"}); err != nil {
		t.Fatalf("Run(inspect) error = %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "pane: default:%5") {
		t.Fatalf("stdout = %q, want pane line", out)
	}
	if !strings.Contains(out, "managed: yes") || !strings.Contains(out, "agent: codex") {
		t.Fatalf("stdout = %q, want metadata lines", out)
	}
	if !strings.Contains(out, "last_activity: "+formatLastActivity(1710000000)) {
		t.Fatalf("stdout = %q, want formatted last activity", out)
	}
}

func TestAppRunSnapshotJSON(t *testing.T) {
	t.Parallel()

	app, stdout, _ := newTestApp(t, func(_ context.Context, _ []byte, args ...string) (tmux.RunResult, error) {
		switch args[0] {
		case "list-panes":
			return tmux.RunResult{Stdout: paneInfoRow()}, nil
		case "capture-pane":
			return tmux.RunResult{Stdout: "line one\nline two\n"}, nil
		default:
			t.Fatalf("unexpected args: %v", args)
			return tmux.RunResult{}, nil
		}
	})

	if err := app.Run(context.Background(), []string{"snapshot", "--pane", "%5", "--lines", "32", "--json"}); err != nil {
		t.Fatalf("Run(snapshot --json) error = %v", err)
	}

	var payload struct {
		Pane     string `json:"pane"`
		Lines    int    `json:"lines"`
		Snapshot string `json:"snapshot"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v\nstdout=%s", err, stdout.String())
	}
	if payload.Pane != "%5" || payload.Lines != 32 || payload.Snapshot != "line one\nline two\n" {
		t.Fatalf("payload = %#v, want snapshot payload", payload)
	}
}

func TestAppRunSendRequiresText(t *testing.T) {
	t.Parallel()

	app, stdout, stderr := newTestApp(t, nil)

	err := app.Run(context.Background(), []string{"send", "--pane", "%5"})
	if ExitCode(err) != ExitUsage {
		t.Fatalf("ExitCode(error) = %d, want %d (err=%v)", ExitCode(err), ExitUsage, err)
	}
	if err == nil || err.Error() != "send requires --text" {
		t.Fatalf("error = %v, want send requires --text", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func newTestApp(t *testing.T, runFn func(context.Context, []byte, ...string) (tmux.RunResult, error)) (*App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	service := NewService(tmux.NewClient(&fakeRunner{runFn: runFn}, ""))
	return NewApp(stdout, stderr, service), stdout, stderr
}

func paneInfoRow() string {
	return strings.Join([]string{
		"%5",
		"dev",
		"@1",
		"editor",
		"api",
		"zsh",
		"/home/gle/project",
		"0",
		"120",
		"40",
	}, "\x1f") + "\n"
}

func paneStateRow() string {
	return strings.Join([]string{
		"%5",
		"dev",
		"@1",
		"editor",
		"api",
		"zsh",
		"/home/gle/project",
		"0",
		"120",
		"40",
		"1",
		string(tmux.ModeRelay),
		string(tmux.AgentCodex),
		"api",
		tmux.CreatedByManualAttach,
		"1710000000",
	}, "\x1f") + "\n"
}

func TestFormatLastActivityZero(t *testing.T) {
	t.Parallel()

	if got := formatLastActivity(0); got != "-" {
		t.Fatalf("formatLastActivity(0) = %q, want -", got)
	}
	if got := formatLastActivity(1710000000); got != time.Unix(1710000000, 0).Format(time.RFC3339) {
		t.Fatalf("formatLastActivity(1710000000) = %q, want RFC3339 timestamp", got)
	}
}

func setBuildInfoForTest(t *testing.T, version string, commit string, date string) func() {
	t.Helper()

	previous := buildinfo.Current()
	buildinfo.Version = version
	buildinfo.Commit = commit
	buildinfo.Date = date

	return func() {
		buildinfo.Version = previous.Version
		buildinfo.Commit = previous.Commit
		buildinfo.Date = previous.Date
	}
}
