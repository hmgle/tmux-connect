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

func TestPrepareFollowMessageDeltaSkipsExactDuplicate(t *testing.T) {
	t.Parallel()

	got, changed := prepareFollowMessageDelta("same output", "same output")
	if changed {
		t.Fatalf("prepareFollowMessageDelta() changed = true, got %q", got)
	}
	if got != "" {
		t.Fatalf("prepareFollowMessageDelta() = %q, want empty", got)
	}
}

func TestPrepareFollowMessageDeltaOmitsRepeatedPrefix(t *testing.T) {
	t.Parallel()

	previous := strings.Join([]string{
		"build step 1 finished successfully",
		"build step 2 finished successfully",
		"waiting for next action",
	}, "\n")
	current := strings.Join([]string{
		"build step 1 finished successfully",
		"build step 2 finished successfully",
		"To continue this session, run /resume",
	}, "\n")

	got, changed := prepareFollowMessageDelta(previous, current)
	if !changed {
		t.Fatal("prepareFollowMessageDelta() changed = false, want true")
	}
	if !strings.HasPrefix(got, followRepeatedPrefixMarker) {
		t.Fatalf("prepareFollowMessageDelta() = %q, want repeated-prefix marker", got)
	}
	if !strings.Contains(got, "To continue this session, run /resume") {
		t.Fatalf("prepareFollowMessageDelta() = %q, want preserved tail", got)
	}
	if strings.Contains(got, "build step 1 finished successfully") {
		t.Fatalf("prepareFollowMessageDelta() = %q, want repeated prefix omitted", got)
	}
}
