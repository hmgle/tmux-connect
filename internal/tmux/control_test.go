package tmux

import (
	"testing"
	"time"
)

func TestDecodeTmuxEscapes(t *testing.T) {
	t.Parallel()

	got, err := decodeTmuxEscapes(`hello\012world\134`)
	if err != nil {
		t.Fatalf("decodeTmuxEscapes() error = %v", err)
	}
	if got != "hello\nworld\\" {
		t.Fatalf("decodeTmuxEscapes() = %q", got)
	}
}

func TestParseNotification(t *testing.T) {
	t.Parallel()

	target := Target{Socket: "default", PaneID: "%5"}
	chunk, ok, err := parseNotification(target, `%output %5 hello\012`)
	if err != nil {
		t.Fatalf("parseNotification() error = %v", err)
	}
	if !ok {
		t.Fatal("expected output notification")
	}
	if chunk.Text != "hello\n" {
		t.Fatalf("chunk text = %q", chunk.Text)
	}
}

func TestParseNotificationReportsMalformedTargetPayload(t *testing.T) {
	t.Parallel()

	target := Target{Socket: "default", PaneID: "%5"}
	_, ok, err := parseNotification(target, `%extended-output %5 0 missing-colon`)
	if ok {
		t.Fatal("did not expect malformed notification to be accepted")
	}
	if err == nil {
		t.Fatal("expected parseNotification() error")
	}
}

func TestParseNotificationExtendedOutput(t *testing.T) {
	t.Parallel()

	target := Target{Socket: "default", PaneID: "%5"}
	chunk, ok, err := parseNotification(target, `%extended-output %5 0 : hello\012world`)
	if err != nil {
		t.Fatalf("parseNotification() error = %v", err)
	}
	if !ok {
		t.Fatal("expected extended output notification")
	}
	if chunk.Text != "hello\nworld" {
		t.Fatalf("chunk text = %q, want %q", chunk.Text, "hello\nworld")
	}
}

func TestCleanControlLineStripsDCSWrapper(t *testing.T) {
	t.Parallel()

	line := "\x1bP1000p%output %5 hello\\012\x1b\\\r\n"
	if got := cleanControlLine(line); got != `%output %5 hello\012` {
		t.Fatalf("cleanControlLine() = %q", got)
	}
}

func TestCutoverSubscriptionTrimsBufferedPrefixPresentInInitialSnapshot(t *testing.T) {
	t.Parallel()

	base := NewSubscriptionForTest()
	oldTime := time.Unix(100, 0)
	newTime := oldTime.Add(time.Second)

	base.PushChunk(OutputChunk{Text: " world", ReceivedAt: oldTime})
	base.PushChunk(OutputChunk{Text: "!", ReceivedAt: newTime})

	wrapped, err := CutoverSubscription(base, "hello world")
	if err != nil {
		t.Fatalf("CutoverSubscription() error = %v", err)
	}

	base.CloseChannels()

	var chunks []OutputChunk
	for chunk := range wrapped.Chunks() {
		chunks = append(chunks, chunk)
	}

	if len(chunks) != 1 {
		t.Fatalf("chunk count = %d, want 1", len(chunks))
	}
	if chunks[0].Text != "!" {
		t.Fatalf("chunk text = %q, want %q", chunks[0].Text, "!")
	}
}
