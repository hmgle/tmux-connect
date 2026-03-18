package daemon

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hmgle/tmux-connect/internal/telegram"
	"golang.org/x/image/font/gofont/gomono"
)

func TestParseConfigAcceptsSnapshotRenderFlags(t *testing.T) {
	t.Parallel()

	fontPath := writeTempFont(t, "mono.ttf")
	cfg, err := parseConfig([]string{
		"--telegram-token", "token",
		"--db", filepath.Join(t.TempDir(), "tagb.db"),
		"--telegram-snapshot-theme", "light",
		"--telegram-snapshot-font-size", "18",
		"--telegram-snapshot-font-file", fontPath,
	}, &bytes.Buffer{}, true)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.SnapshotTheme != "light" {
		t.Fatalf("snapshot theme = %q, want light", cfg.SnapshotTheme)
	}
	if cfg.SnapshotFontSize != 18 {
		t.Fatalf("snapshot font size = %v, want 18", cfg.SnapshotFontSize)
	}
	if cfg.SnapshotFontFile != fontPath {
		t.Fatalf("snapshot font file = %q, want %q", cfg.SnapshotFontFile, fontPath)
	}
}

func TestParseConfigReadsSnapshotRenderEnv(t *testing.T) {
	fontPath := writeTempFont(t, "env-font.ttf")
	t.Setenv("TAGB_TELEGRAM_TOKEN", "token")
	t.Setenv("TAGB_TELEGRAM_SNAPSHOT_THEME", "light")
	t.Setenv("TAGB_TELEGRAM_SNAPSHOT_FONT_SIZE", "16.5")
	t.Setenv("TAGB_TELEGRAM_SNAPSHOT_FONT_FILE", fontPath)

	cfg, err := parseConfig([]string{"--db", filepath.Join(t.TempDir(), "tagb.db")}, &bytes.Buffer{}, true)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.SnapshotTheme != "light" {
		t.Fatalf("snapshot theme = %q, want light", cfg.SnapshotTheme)
	}
	if cfg.SnapshotFontSize != 16.5 {
		t.Fatalf("snapshot font size = %v, want 16.5", cfg.SnapshotFontSize)
	}
	if cfg.SnapshotFontFile != fontPath {
		t.Fatalf("snapshot font file = %q, want %q", cfg.SnapshotFontFile, fontPath)
	}
}

func TestParseConfigRejectsInvalidSnapshotTheme(t *testing.T) {
	t.Parallel()

	_, err := parseConfig([]string{
		"--telegram-token", "token",
		"--db", filepath.Join(t.TempDir(), "tagb.db"),
		"--telegram-snapshot-theme", "sepia",
	}, &bytes.Buffer{}, true)
	if err == nil {
		t.Fatal("parseConfig() error = nil, want error")
	}
}

func TestParseConfigRejectsInvalidSnapshotFontSizeEnv(t *testing.T) {
	t.Setenv("TAGB_TELEGRAM_SNAPSHOT_FONT_SIZE", "large")
	_, err := parseConfig([]string{
		"--telegram-token", "token",
		"--db", filepath.Join(t.TempDir(), "tagb.db"),
	}, &bytes.Buffer{}, true)
	if err == nil {
		t.Fatal("parseConfig() error = nil, want error")
	}
}

func TestParseConfigRejectsInvalidSnapshotFontExtension(t *testing.T) {
	t.Parallel()

	fontPath := filepath.Join(t.TempDir(), "mono.txt")
	if err := os.WriteFile(fontPath, []byte("not a font"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	_, err := parseConfig([]string{
		"--telegram-token", "token",
		"--db", filepath.Join(t.TempDir(), "tagb.db"),
		"--telegram-snapshot-font-file", fontPath,
	}, &bytes.Buffer{}, true)
	if err == nil {
		t.Fatal("parseConfig() error = nil, want error")
	}
}

type fakeTelegramBot struct {
	setCommandsErr error
	getUpdatesErr  error
	updates        []telegram.Update
	getUpdatesFn   func(context.Context, int64, time.Duration) ([]telegram.Update, error)
	order          []string
	commands       []telegram.BotCommand
}

func (f *fakeTelegramBot) SendMessage(context.Context, int64, string, telegram.SendOptions) (telegram.Message, error) {
	return telegram.Message{}, nil
}

func (f *fakeTelegramBot) SendPhoto(context.Context, int64, string, []byte, string, telegram.SendOptions) (telegram.Message, error) {
	return telegram.Message{}, nil
}

func (f *fakeTelegramBot) PollTimeout() time.Duration {
	return time.Second
}

func (f *fakeTelegramBot) DrainPendingUpdates(context.Context) (int64, error) {
	f.order = append(f.order, "drain")
	return 0, nil
}

func (f *fakeTelegramBot) GetUpdates(ctx context.Context, offset int64, timeout time.Duration) ([]telegram.Update, error) {
	f.order = append(f.order, "get")
	if f.getUpdatesFn != nil {
		return f.getUpdatesFn(ctx, offset, timeout)
	}
	return f.updates, f.getUpdatesErr
}

func (f *fakeTelegramBot) SetMyCommands(_ context.Context, commands []telegram.BotCommand) error {
	f.order = append(f.order, "set")
	f.commands = append([]telegram.BotCommand(nil), commands...)
	return f.setCommandsErr
}

func TestRuntimeRunRegistersTelegramCommandsBeforePolling(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	bot := &fakeTelegramBot{}
	runtime := &Runtime{
		client: bot,
		stderr: &bytes.Buffer{},
	}
	bot.getUpdatesFn = func(_ context.Context, _ int64, _ time.Duration) ([]telegram.Update, error) {
		cancel()
		return nil, context.Canceled
	}

	if err := runtime.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if strings.Join(bot.order, ",") != "set,drain,get" {
		t.Fatalf("call order = %q, want %q", strings.Join(bot.order, ","), "set,drain,get")
	}
	if len(bot.commands) != len(daemonCommandSpecs()) {
		t.Fatalf("commands len = %d, want %d", len(bot.commands), len(daemonCommandSpecs()))
	}
	if bot.commands[0].Command != "start" {
		t.Fatalf("first command = %#v, want start", bot.commands[0])
	}
}

func TestRuntimeRunReturnsMenuRegistrationError(t *testing.T) {
	t.Parallel()

	bot := &fakeTelegramBot{setCommandsErr: context.DeadlineExceeded}
	runtime := &Runtime{
		client: bot,
		stderr: &bytes.Buffer{},
	}
	err := runtime.Run(context.Background())
	if err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "configure telegram commands") {
		t.Fatalf("Run() error = %q, want configure telegram commands", err)
	}
	if strings.Join(bot.order, ",") != "set" {
		t.Fatalf("call order = %q, want %q", strings.Join(bot.order, ","), "set")
	}
}

func writeTempFont(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, gomono.TTF, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
