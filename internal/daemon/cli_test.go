package daemon

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hmgle/tmux-connect/internal/config"
	"golang.org/x/image/font/gofont/gomono"
)

func TestParseConfigAcceptsSnapshotRenderFlags(t *testing.T) {
	t.Parallel()

	fontPath := writeTempFont(t, "mono.ttf")
	cfg, err := parseConfig([]string{
		"--telegram-token", "token",
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
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
	t.Setenv("TMUXCONN_TELEGRAM_TOKEN", "token")
	t.Setenv("TMUXCONN_TELEGRAM_SNAPSHOT_THEME", "light")
	t.Setenv("TMUXCONN_TELEGRAM_SNAPSHOT_FONT_SIZE", "16.5")
	t.Setenv("TMUXCONN_TELEGRAM_SNAPSHOT_FONT_FILE", fontPath)

	cfg, err := parseConfig([]string{"--db", filepath.Join(t.TempDir(), "tmuxconn.db")}, &bytes.Buffer{}, true)
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
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
		"--telegram-snapshot-theme", "sepia",
	}, &bytes.Buffer{}, true)
	if err == nil {
		t.Fatal("parseConfig() error = nil, want error")
	}
}

func TestParseConfigRejectsInvalidSnapshotFontSizeEnv(t *testing.T) {
	t.Setenv("TMUXCONN_TELEGRAM_SNAPSHOT_FONT_SIZE", "large")
	_, err := parseConfig([]string{
		"--telegram-token", "token",
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
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
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
		"--telegram-snapshot-font-file", fontPath,
	}, &bytes.Buffer{}, true)
	if err == nil {
		t.Fatal("parseConfig() error = nil, want error")
	}
}

func TestParseConfigReadsFileDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := parseConfigWithFile(nil, &bytes.Buffer{}, true, config.Daemon{
		DB:                   stringPtr(filepath.Join(t.TempDir(), "tmuxconn.db")),
		Platform:             stringPtr("telegram"),
		AllowChats:           &[]string{"123", "456"},
		PollTimeout:          stringPtr("30s"),
		SnapshotLines:        intPtr(150),
		PlainTextMode:        stringPtr("execute"),
		PlainTextEcho:        stringPtr("snapshot"),
		PlainTextEchoLines:   intPtr(9),
		PlainTextEchoDelay:   stringPtr("400ms"),
		PlainTextEchoTimeout: stringPtr("3s"),
		FollowLines:          intPtr(90),
		Telegram: config.Telegram{
			Token:            stringPtr("file-token"),
			SnapshotTheme:    stringPtr("light"),
			SnapshotFontSize: float64Ptr(18),
		},
	})
	if err != nil {
		t.Fatalf("parseConfigWithFile() error = %v", err)
	}
	if cfg.TelegramToken != "file-token" {
		t.Fatalf("TelegramToken = %q, want file-token", cfg.TelegramToken)
	}
	if cfg.PollTimeout.String() != "30s" {
		t.Fatalf("PollTimeout = %s, want 30s", cfg.PollTimeout)
	}
	if cfg.SnapshotLines != 150 {
		t.Fatalf("SnapshotLines = %d, want 150", cfg.SnapshotLines)
	}
	if cfg.PlainTextMode != plainTextModeExecute {
		t.Fatalf("PlainTextMode = %q, want execute", cfg.PlainTextMode)
	}
	if cfg.PlainTextEcho != plainTextEchoSnapshot {
		t.Fatalf("PlainTextEcho = %q, want snapshot", cfg.PlainTextEcho)
	}
	if cfg.PlainTextEchoLines != 9 {
		t.Fatalf("PlainTextEchoLines = %d, want 9", cfg.PlainTextEchoLines)
	}
	if cfg.PlainTextEchoDelay != 400*time.Millisecond {
		t.Fatalf("PlainTextEchoDelay = %s, want 400ms", cfg.PlainTextEchoDelay)
	}
	if cfg.PlainTextEchoTimeout != 3*time.Second {
		t.Fatalf("PlainTextEchoTimeout = %s, want 3s", cfg.PlainTextEchoTimeout)
	}
	if cfg.FollowLines != 90 {
		t.Fatalf("FollowLines = %d, want 90", cfg.FollowLines)
	}
	if cfg.SnapshotTheme != "light" {
		t.Fatalf("SnapshotTheme = %q, want light", cfg.SnapshotTheme)
	}
	if cfg.SnapshotFontSize != 18 {
		t.Fatalf("SnapshotFontSize = %v, want 18", cfg.SnapshotFontSize)
	}
	if strings.Join(cfg.AllowChats, ",") != "123,456" {
		t.Fatalf("AllowChats = %#v, want [123 456]", cfg.AllowChats)
	}
}

func TestParseConfigEnvOverridesFile(t *testing.T) {
	t.Setenv("TMUXCONN_TELEGRAM_TOKEN", "env-token")
	t.Setenv("TMUXCONN_TELEGRAM_SNAPSHOT_FONT_SIZE", "16.5")
	t.Setenv("TMUXCONN_PLAIN_TEXT_MODE", "execute")
	t.Setenv("TMUXCONN_PLAIN_TEXT_ECHO_LINES", "7")

	cfg, err := parseConfigWithFile([]string{"--db", filepath.Join(t.TempDir(), "tmuxconn.db")}, &bytes.Buffer{}, true, config.Daemon{
		Telegram: config.Telegram{
			Token:            stringPtr("file-token"),
			SnapshotFontSize: float64Ptr(18),
		},
		PlainTextMode:      stringPtr("type"),
		PlainTextEchoLines: intPtr(12),
	})
	if err != nil {
		t.Fatalf("parseConfigWithFile() error = %v", err)
	}
	if cfg.TelegramToken != "env-token" {
		t.Fatalf("TelegramToken = %q, want env-token", cfg.TelegramToken)
	}
	if cfg.SnapshotFontSize != 16.5 {
		t.Fatalf("SnapshotFontSize = %v, want 16.5", cfg.SnapshotFontSize)
	}
	if cfg.PlainTextMode != plainTextModeExecute {
		t.Fatalf("PlainTextMode = %q, want execute", cfg.PlainTextMode)
	}
	if cfg.PlainTextEchoLines != 7 {
		t.Fatalf("PlainTextEchoLines = %d, want 7", cfg.PlainTextEchoLines)
	}
}

func TestParseConfigFlagsOverrideFile(t *testing.T) {
	t.Parallel()

	cfg, err := parseConfigWithFile([]string{
		"--telegram-token", "flag-token",
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
		"--telegram-snapshot-theme", "light",
		"--plain-text-mode", "execute",
		"--plain-text-echo", "off",
		"--plain-text-echo-lines", "5",
		"--plain-text-echo-delay", "150ms",
		"--plain-text-echo-timeout", "1s",
	}, &bytes.Buffer{}, true, config.Daemon{
		Telegram: config.Telegram{
			Token:         stringPtr("file-token"),
			SnapshotTheme: stringPtr("dark"),
		},
		PlainTextMode:        stringPtr("type"),
		PlainTextEcho:        stringPtr("snapshot"),
		PlainTextEchoLines:   intPtr(12),
		PlainTextEchoDelay:   stringPtr("250ms"),
		PlainTextEchoTimeout: stringPtr("2s"),
	})
	if err != nil {
		t.Fatalf("parseConfigWithFile() error = %v", err)
	}
	if cfg.TelegramToken != "flag-token" {
		t.Fatalf("TelegramToken = %q, want flag-token", cfg.TelegramToken)
	}
	if cfg.SnapshotTheme != "light" {
		t.Fatalf("SnapshotTheme = %q, want light", cfg.SnapshotTheme)
	}
	if cfg.PlainTextMode != plainTextModeExecute {
		t.Fatalf("PlainTextMode = %q, want execute", cfg.PlainTextMode)
	}
	if cfg.PlainTextEcho != plainTextEchoOff {
		t.Fatalf("PlainTextEcho = %q, want off", cfg.PlainTextEcho)
	}
	if cfg.PlainTextEchoLines != 5 {
		t.Fatalf("PlainTextEchoLines = %d, want 5", cfg.PlainTextEchoLines)
	}
	if cfg.PlainTextEchoDelay != 150*time.Millisecond {
		t.Fatalf("PlainTextEchoDelay = %s, want 150ms", cfg.PlainTextEchoDelay)
	}
	if cfg.PlainTextEchoTimeout != time.Second {
		t.Fatalf("PlainTextEchoTimeout = %s, want 1s", cfg.PlainTextEchoTimeout)
	}
}

func TestParseConfigRejectsInvalidPlainTextMode(t *testing.T) {
	t.Parallel()

	_, err := parseConfig([]string{
		"--telegram-token", "token",
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
		"--plain-text-mode", "run",
	}, &bytes.Buffer{}, true)
	if err == nil {
		t.Fatal("parseConfig() error = nil, want error")
	}
}

func TestParseConfigRejectsInvalidPlainTextEchoLines(t *testing.T) {
	t.Parallel()

	_, err := parseConfig([]string{
		"--telegram-token", "token",
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
		"--plain-text-echo-lines", "0",
	}, &bytes.Buffer{}, true)
	if err == nil {
		t.Fatal("parseConfig() error = nil, want error")
	}
}

func TestParseConfigRejectsInvalidPlainTextEchoDelay(t *testing.T) {
	t.Parallel()

	_, err := parseConfigWithFile([]string{
		"--telegram-token", "token",
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
	}, &bytes.Buffer{}, true, config.Daemon{
		PlainTextEchoDelay: stringPtr("soon"),
	})
	if err == nil {
		t.Fatal("parseConfigWithFile() error = nil, want error")
	}
}

func TestParseConfigAllowChatFlagOverridesFile(t *testing.T) {
	t.Parallel()

	cfg, err := parseConfigWithFile([]string{
		"--telegram-token", "token",
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
		"--allow-chat", "999",
		"--allow-chat", "888",
	}, &bytes.Buffer{}, true, config.Daemon{
		AllowChats: &[]string{"123", "456"},
	})
	if err != nil {
		t.Fatalf("parseConfigWithFile() error = %v", err)
	}
	if strings.Join(cfg.AllowChats, ",") != "999,888" {
		t.Fatalf("AllowChats = %#v, want [999 888]", cfg.AllowChats)
	}
}

func TestParseConfigRejectsInvalidFileDuration(t *testing.T) {
	t.Parallel()

	_, err := parseConfigWithFile([]string{
		"--telegram-token", "token",
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
	}, &bytes.Buffer{}, true, config.Daemon{
		PollTimeout: stringPtr("later"),
	})
	if err == nil {
		t.Fatal("parseConfigWithFile() error = nil, want error")
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
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
	}, &bytes.Buffer{}, true)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.SlackCommandPrefix != "bot:" {
		t.Fatalf("SlackCommandPrefix = %q, want %q", cfg.SlackCommandPrefix, "bot:")
	}
}

func TestParseConfigSlackCommandPrefixEnv(t *testing.T) {
	t.Setenv("TMUXCONN_PLATFORM", "slack")
	t.Setenv("TMUXCONN_SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("TMUXCONN_SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("TMUXCONN_SLACK_COMMAND_PREFIX", "run:")

	cfg, err := parseConfig([]string{
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
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
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
	}, &bytes.Buffer{}, true)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.SlackCommandPrefix != defaultSlackCommandPrefix {
		t.Fatalf("SlackCommandPrefix = %q, want %q", cfg.SlackCommandPrefix, defaultSlackCommandPrefix)
	}
}

func TestParseConfigDiscordCommandPrefixFlag(t *testing.T) {
	t.Parallel()

	cfg, err := parseConfig([]string{
		"--platform", "discord",
		"--discord-token", "discord-token",
		"--discord-command-prefix", "bot:",
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
	}, &bytes.Buffer{}, true)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.DiscordCommandPrefix != "bot:" {
		t.Fatalf("DiscordCommandPrefix = %q, want %q", cfg.DiscordCommandPrefix, "bot:")
	}
}

func TestParseConfigDiscordCommandPrefixDefault(t *testing.T) {
	t.Parallel()

	cfg, err := parseConfig([]string{
		"--platform", "discord",
		"--discord-token", "discord-token",
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
	}, &bytes.Buffer{}, true)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.DiscordCommandPrefix != defaultDiscordCommandPrefix {
		t.Fatalf("DiscordCommandPrefix = %q, want %q", cfg.DiscordCommandPrefix, defaultDiscordCommandPrefix)
	}
}

func TestParseConfigRejectsWhitespaceDiscordCommandPrefix(t *testing.T) {
	t.Parallel()

	_, err := parseConfig([]string{
		"--platform", "discord",
		"--discord-token", "discord-token",
		"--discord-command-prefix", "bad prefix",
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
	}, &bytes.Buffer{}, true)
	if err == nil {
		t.Fatal("parseConfig() error = nil, want error")
	}
}

func TestParseConfigRejectsEmptySlackCommandPrefix(t *testing.T) {
	t.Parallel()

	_, err := parseConfig([]string{
		"--platform", "slack",
		"--slack-bot-token", "xoxb-test",
		"--slack-app-token", "xapp-test",
		"--slack-command-prefix", "",
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
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
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
	}, &bytes.Buffer{}, true)
	if err == nil {
		t.Fatal("parseConfig() error = nil, want error for whitespace in prefix")
	}
}

func TestParseConfigTelegramClearsSlackPrefixEnv(t *testing.T) {
	t.Setenv("TMUXCONN_TELEGRAM_TOKEN", "token")
	t.Setenv("TMUXCONN_SLACK_COMMAND_PREFIX", "bad prefix")

	cfg, err := parseConfig([]string{
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
	}, &bytes.Buffer{}, true)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.Platform != "telegram" {
		t.Fatalf("platform = %q, want telegram", cfg.Platform)
	}
	if cfg.SlackCommandPrefix != "" {
		t.Fatalf("SlackCommandPrefix = %q, want empty for telegram platform", cfg.SlackCommandPrefix)
	}
}

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

func writeTempFont(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, gomono.TTF, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func stringPtr(value string) *string {
	return &value
}

func intPtr(value int) *int {
	return &value
}

func float64Ptr(value float64) *float64 {
	return &value
}
