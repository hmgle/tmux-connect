package daemon

import (
	"testing"
)

func TestDecorateTelegramMessageForTerminalOutput(t *testing.T) {
	t.Parallel()

	text, opts := decorateTelegramMessage("snapshot", "[default:%5]\n<ready> & ok", SendOptions{})
	if opts.Format != MessageFormatTelegramHTML {
		t.Fatalf("format = %q, want %q", opts.Format, MessageFormatTelegramHTML)
	}

	want := "<b>[default:%5]</b>\n<pre>&lt;ready&gt; &amp; ok</pre>"
	if text != want {
		t.Fatalf("text = %q, want %q", text, want)
	}
}

func TestDecorateTelegramMessageLeavesNonTerminalTextPlain(t *testing.T) {
	t.Parallel()

	text, opts := decorateTelegramMessage("help", "Commands:\n/panes", SendOptions{})
	if text != "Commands:\n/panes" {
		t.Fatalf("text = %q, want plain text", text)
	}
	if opts.Format != "" {
		t.Fatalf("format = %q, want empty", opts.Format)
	}
}

func TestDecorateTelegramMessagePanesPassthroughHTML(t *testing.T) {
	t.Parallel()

	input := "<b>Panes:</b>\n<pre>> %5  bash  project</pre>\nCurrent: %5 · Follow: off"
	text, opts := decorateTelegramMessage("panes", input, SendOptions{})
	if opts.Format != MessageFormatTelegramHTML {
		t.Fatalf("format = %q, want %q", opts.Format, MessageFormatTelegramHTML)
	}
	if text != input {
		t.Fatalf("text = %q, want passthrough", text)
	}
}
