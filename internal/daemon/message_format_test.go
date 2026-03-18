package daemon

import (
	"testing"

	"github.com/hmgle/tmux-connect/internal/telegram"
)

func TestDecorateTelegramMessageForTerminalOutput(t *testing.T) {
	t.Parallel()

	text, opts := decorateTelegramMessage("snapshot", "[default:%5]\n<ready> & ok", telegram.SendOptions{})
	if opts.ParseMode != telegram.ParseModeHTML {
		t.Fatalf("parse mode = %q, want %q", opts.ParseMode, telegram.ParseModeHTML)
	}

	want := "<b>[default:%5]</b>\n<pre>&lt;ready&gt; &amp; ok</pre>"
	if text != want {
		t.Fatalf("text = %q, want %q", text, want)
	}
}

func TestDecorateTelegramMessageLeavesNonTerminalTextPlain(t *testing.T) {
	t.Parallel()

	text, opts := decorateTelegramMessage("help", "Commands:\n/panes", telegram.SendOptions{})
	if text != "Commands:\n/panes" {
		t.Fatalf("text = %q, want plain text", text)
	}
	if opts.ParseMode != "" {
		t.Fatalf("parse mode = %q, want empty", opts.ParseMode)
	}
}

func TestDecorateTelegramMessagePanesPassthroughHTML(t *testing.T) {
	t.Parallel()

	input := "<b>Panes:</b>\n<pre>> %5  bash  project</pre>\nCurrent: %5 · Follow: off"
	text, opts := decorateTelegramMessage("panes", input, telegram.SendOptions{})
	if opts.ParseMode != telegram.ParseModeHTML {
		t.Fatalf("parse mode = %q, want %q", opts.ParseMode, telegram.ParseModeHTML)
	}
	if text != input {
		t.Fatalf("text = %q, want passthrough", text)
	}
}
