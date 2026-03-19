package daemon

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

type fakeRuntimeAdapter struct {
	registerErr error
	runErr      error
	runFn       func(context.Context, func(context.Context, IncomingMessage) error) error
	order       []string
	commands    []botCommandSpec
}

func (f *fakeRuntimeAdapter) Platform() string { return "telegram" }
func (f *fakeRuntimeAdapter) SendMessage(context.Context, ChatRef, string, SendOptions) (OutboundMessage, error) {
	return OutboundMessage{}, nil
}
func (f *fakeRuntimeAdapter) SendImage(context.Context, ChatRef, string, []byte, string, SendOptions) (OutboundMessage, error) {
	return OutboundMessage{}, nil
}
func (f *fakeRuntimeAdapter) DecorateMessage(kind string, text string, opts SendOptions) (string, SendOptions) {
	return text, opts
}
func (f *fakeRuntimeAdapter) PromptOptions(IncomingMessage, commandPromptSpec) SendOptions {
	return SendOptions{}
}
func (f *fakeRuntimeAdapter) SnapshotCaption(string) string { return "" }
func (f *fakeRuntimeAdapter) Run(ctx context.Context, handler func(context.Context, IncomingMessage) error) error {
	f.order = append(f.order, "run")
	if f.runFn != nil {
		return f.runFn(ctx, handler)
	}
	return f.runErr
}
func (f *fakeRuntimeAdapter) RegisterCommands(_ context.Context, commands []botCommandSpec) error {
	f.order = append(f.order, "set")
	f.commands = append([]botCommandSpec(nil), commands...)
	return f.registerErr
}
func (f *fakeRuntimeAdapter) Close() error { return nil }

func TestRuntimeRunRegistersTelegramCommandsBeforePolling(t *testing.T) {
	t.Parallel()

	bot := &fakeRuntimeAdapter{}
	runtime := &Runtime{
		adapter: bot,
		stderr:  &bytes.Buffer{},
	}

	if err := runtime.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if strings.Join(bot.order, ",") != "set,run" {
		t.Fatalf("call order = %q, want %q", strings.Join(bot.order, ","), "set,run")
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

	bot := &fakeRuntimeAdapter{registerErr: context.DeadlineExceeded}
	runtime := &Runtime{
		adapter: bot,
		stderr:  &bytes.Buffer{},
	}
	err := runtime.Run(context.Background())
	if err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("Run() error = %q, want deadline exceeded", err)
	}
	if strings.Join(bot.order, ",") != "set" {
		t.Fatalf("call order = %q, want %q", strings.Join(bot.order, ","), "set")
	}
}

func TestParseConfigSlackCommandPrefixFlag(t *testing.T) {
	t.Parallel()

	cfg, err := parseConfig([]string{
		"--platform", "slack",
		"--slack-bot-token", "xoxb-test",
		"--slack-app-token", "xapp-test",
		"--slack-command-prefix", "bot:",
		"--db", filepath.Join(t.TempDir(), "tagb.db"),
	}, &bytes.Buffer{}, true)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.SlackCommandPrefix != "bot:" {
		t.Fatalf("SlackCommandPrefix = %q, want %q", cfg.SlackCommandPrefix, "bot:")
	}
}

func TestParseConfigSlackCommandPrefixEnv(t *testing.T) {
	t.Setenv("TAGB_PLATFORM", "slack")
	t.Setenv("TAGB_SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("TAGB_SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("TAGB_SLACK_COMMAND_PREFIX", "run:")

	cfg, err := parseConfig([]string{
		"--db", filepath.Join(t.TempDir(), "tagb.db"),
	}, &bytes.Buffer{}, true)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.SlackCommandPrefix != "run:" {
		t.Fatalf("SlackCommandPrefix = %q, want %q", cfg.SlackCommandPrefix, "run:")
	}
}

func TestParseConfigSlackCommandPrefixDefault(t *testing.T) {
	t.Parallel()

	cfg, err := parseConfig([]string{
		"--platform", "slack",
		"--slack-bot-token", "xoxb-test",
		"--slack-app-token", "xapp-test",
		"--db", filepath.Join(t.TempDir(), "tagb.db"),
	}, &bytes.Buffer{}, true)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.SlackCommandPrefix != defaultSlackCommandPrefix {
		t.Fatalf("SlackCommandPrefix = %q, want %q", cfg.SlackCommandPrefix, defaultSlackCommandPrefix)
	}
}

func TestParseConfigRejectsEmptySlackCommandPrefix(t *testing.T) {
	t.Parallel()

	_, err := parseConfig([]string{
		"--platform", "slack",
		"--slack-bot-token", "xoxb-test",
		"--slack-app-token", "xapp-test",
		"--slack-command-prefix", "",
		"--db", filepath.Join(t.TempDir(), "tagb.db"),
	}, &bytes.Buffer{}, true)
	if err == nil {
		t.Fatal("parseConfig() error = nil, want error for empty prefix")
	}
}

func TestParseConfigRejectsWhitespaceSlackCommandPrefix(t *testing.T) {
	t.Parallel()

	_, err := parseConfig([]string{
		"--platform", "slack",
		"--slack-bot-token", "xoxb-test",
		"--slack-app-token", "xapp-test",
		"--slack-command-prefix", "my prefix",
		"--db", filepath.Join(t.TempDir(), "tagb.db"),
	}, &bytes.Buffer{}, true)
	if err == nil {
		t.Fatal("parseConfig() error = nil, want error for whitespace in prefix")
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
