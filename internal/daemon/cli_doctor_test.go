package daemon

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hmgle/tmux-connect/internal/config"
)

func TestRunDoctorSlackPrintsSnapshotUploadHints(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	err := runDoctorWithConfig(context.Background(), stdout, &bytes.Buffer{}, newFakePaneService(), config.Daemon{}, []string{
		"--platform", "slack",
		"--slack-bot-token", "xoxb-test",
		"--slack-app-token", "xapp-test",
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
	})
	if err != nil {
		t.Fatalf("runDoctor() error = %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"slack tokens: ok",
		"files:write",
		"reinstall the app",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("runDoctor() output = %q, want %q", output, want)
		}
	}
}

func TestRunDoctorDiscordPrintsIntentHint(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	err := runDoctorWithConfig(context.Background(), stdout, &bytes.Buffer{}, newFakePaneService(), config.Daemon{}, []string{
		"--platform", "discord",
		"--discord-token", "discord-token",
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
	})
	if err != nil {
		t.Fatalf("runDoctor() error = %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"discord token: ok",
		"Message Content intent",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("runDoctor() output = %q, want %q", output, want)
		}
	}
}

func TestRunDoctorWhatsAppPrintsLoginHint(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	err := runDoctorWithConfig(context.Background(), stdout, &bytes.Buffer{}, newFakePaneService(), config.Daemon{}, []string{
		"--platform", "whatsapp",
		"--whatsapp-session-db", filepath.Join(t.TempDir(), "whatsapp-device.db"),
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
	})
	if err != nil {
		t.Fatalf("runDoctor() error = %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"whatsapp session db: ok",
		"pairing QR code",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("runDoctor() output = %q, want %q", output, want)
		}
	}
}
