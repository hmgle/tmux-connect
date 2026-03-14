package tagb

import "testing"

func TestParseTarget(t *testing.T) {
	t.Parallel()

	target, err := ParseTarget("%5", "")
	if err != nil {
		t.Fatalf("ParseTarget returned error: %v", err)
	}
	if got, want := target.Socket, DefaultSocket; got != want {
		t.Fatalf("socket = %q, want %q", got, want)
	}
	if got, want := target.PaneKey(), "default:%5"; got != want {
		t.Fatalf("pane key = %q, want %q", got, want)
	}

	target, err = ParseTarget("work:%9", "")
	if err != nil {
		t.Fatalf("ParseTarget returned error: %v", err)
	}
	if got, want := target.Socket, "work"; got != want {
		t.Fatalf("socket = %q, want %q", got, want)
	}
}

func TestParseTargetRejectsBadPaneID(t *testing.T) {
	t.Parallel()

	if _, err := ParseTarget("pane-1", ""); err == nil {
		t.Fatal("expected an error for invalid pane ID")
	}
}
