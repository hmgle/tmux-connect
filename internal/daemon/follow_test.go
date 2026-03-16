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

func TestBuildFollowUpdateSkipsExactDuplicate(t *testing.T) {
	t.Parallel()

	got, changed := buildFollowUpdate("same output", "same output")
	if changed {
		t.Fatalf("buildFollowUpdate() changed = true, got %q", got)
	}
	if got != "" {
		t.Fatalf("buildFollowUpdate() = %q, want empty", got)
	}
}

func TestBuildFollowUpdateShowsInlineContext(t *testing.T) {
	t.Parallel()

	previous := strings.Join([]string{
		"ready",
		"calc> ",
	}, "\n")
	current := previous + "1+2"

	got, changed := buildFollowUpdate(previous, current)
	if !changed {
		t.Fatal("buildFollowUpdate() changed = false, want true")
	}
	if !strings.Contains(got, "ready") {
		t.Fatalf("buildFollowUpdate() = %q, want prior context", got)
	}
	if !strings.Contains(got, "calc> 1+2") {
		t.Fatalf("buildFollowUpdate() = %q, want full updated line", got)
	}
	if strings.Contains(got, "\n1+2") {
		t.Fatalf("buildFollowUpdate() = %q, want contextual line instead of bare suffix", got)
	}
}

func TestBuildFollowUpdateReturnsDeltaForCompletedLines(t *testing.T) {
	t.Parallel()

	previous := "line one\n"
	current := "line one\nline two\nline three\n"

	got, changed := buildFollowUpdate(previous, current)
	if !changed {
		t.Fatal("buildFollowUpdate() changed = false, want true")
	}
	want := "line two\nline three"
	if got != want {
		t.Fatalf("buildFollowUpdate() = %q, want %q", got, want)
	}
}
