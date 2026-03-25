package daemon

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

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
func (f *fakeRuntimeAdapter) ParseMessage(message IncomingMessage) parsedCommand {
	return defaultParseMessage(message, "")
}
func (f *fakeRuntimeAdapter) PromptOptions(IncomingMessage, commandPromptSpec) SendOptions {
	return SendOptions{}
}
func (f *fakeRuntimeAdapter) PromptText(_ IncomingMessage, spec commandPromptSpec) string {
	return spec.Message
}
func (f *fakeRuntimeAdapter) NormalizeSnapshotMode(mode snapshotMode) snapshotMode {
	return mode
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
