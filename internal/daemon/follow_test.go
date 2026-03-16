package daemon

import (
	"strings"
	"testing"
)

func TestFormatFollowMessageKeepsTailWhenTruncated(t *testing.T) {
	t.Parallel()

	text := strings.Join([]string{
		"header line",
		"noise line 1",
		"noise line 2",
		"To continue this session, run /resume",
	}, "\n")

	got := formatFollowMessage("default:%5", text, 60)

	if !strings.HasPrefix(got, "[default:%5]\n...[truncated]\n") {
		t.Fatalf("formatFollowMessage() prefix = %q", got)
	}
	if !strings.Contains(got, "To continue this session, run /resume") {
		t.Fatalf("formatFollowMessage() = %q, want preserved tail", got)
	}
	if strings.Contains(got, "header line") {
		t.Fatalf("formatFollowMessage() = %q, want leading content trimmed", got)
	}
}

func TestFormatFollowMessageEmptyAfterTrim(t *testing.T) {
	t.Parallel()

	got := formatFollowMessage("default:%5", "  \n\t ", 20)
	want := "[default:%5] (empty output)"
	if got != want {
		t.Fatalf("formatFollowMessage() = %q, want %q", got, want)
	}
}
