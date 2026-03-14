package tmux

import "testing"

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
	chunk, ok := parseNotification(target, `%output %5 hello\012`)
	if !ok {
		t.Fatal("expected output notification")
	}
	if chunk.Text != "hello\n" {
		t.Fatalf("chunk text = %q", chunk.Text)
	}
}
