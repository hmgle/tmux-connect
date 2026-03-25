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
	requirePlatformAvailable(t, "slack")

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

func TestRunDoctorFeishuPrintsSetupHints(t *testing.T) {
	t.Parallel()
	requirePlatformAvailable(t, "feishu")

	stdout := &bytes.Buffer{}
	err := runDoctorWithConfig(context.Background(), stdout, &bytes.Buffer{}, newFakePaneService(), config.Daemon{}, []string{
		"--platform", "feishu",
		"--feishu-app-id", "cli_test",
		"--feishu-app-secret", "secret_test",
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
	})
	if err != nil {
		t.Fatalf("runDoctor() error = %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"feishu app credentials: ok",
		"im.message.receive_v1",
		"@mentions",
		"feishu bot identity: optional",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("runDoctor() output = %q, want %q", output, want)
		}
	}
}

func TestRunDoctorFeishuPrintsBotIdentityConfiguredHint(t *testing.T) {
	t.Parallel()
	requirePlatformAvailable(t, "feishu")

	stdout := &bytes.Buffer{}
	err := runDoctorWithConfig(context.Background(), stdout, &bytes.Buffer{}, newFakePaneService(), config.Daemon{}, []string{
		"--platform", "feishu",
		"--feishu-app-id", "cli_test",
		"--feishu-app-secret", "secret_test",
		"--feishu-bot-open-id", "ou_bot",
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
	})
	if err != nil {
		t.Fatalf("runDoctor() error = %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "precise group @mention matching enabled") {
		t.Fatalf("runDoctor() output = %q, want precise bot identity hint", got)
	}
}

func TestRunDoctorDiscordPrintsIntentHint(t *testing.T) {
	t.Parallel()
	requirePlatformAvailable(t, "discord")

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
	requirePlatformAvailable(t, "whatsapp")

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

func TestRunDoctorWeixinPrintsContextHint(t *testing.T) {
	t.Parallel()
	requirePlatformAvailable(t, "weixin")

	stdout := &bytes.Buffer{}
	err := runDoctorWithConfig(context.Background(), stdout, &bytes.Buffer{}, newFakePaneService(), config.Daemon{}, []string{
		"--platform", "weixin",
		"--weixin-token", "test-token",
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
	})
	if err != nil {
		t.Fatalf("runDoctor() error = %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"weixin token: ok",
		"weixin first message",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("runDoctor() output = %q, want %q", output, want)
		}
	}
}
