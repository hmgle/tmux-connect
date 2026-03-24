package daemon

import "testing"

func TestThreadReplyTargetPrefersExplicitThreadID(t *testing.T) {
	t.Parallel()

	got := threadReplyTarget(SendOptions{
		ThreadID:         "thread-1",
		ReplyToMessageID: "reply-1",
	})
	if got != "thread-1" {
		t.Fatalf("threadReplyTarget() = %q, want thread-1", got)
	}
}

func TestThreadReplyTargetFallsBackToReplyMessageID(t *testing.T) {
	t.Parallel()

	got := threadReplyTarget(SendOptions{ReplyToMessageID: "reply-1"})
	if got != "reply-1" {
		t.Fatalf("threadReplyTarget() = %q, want reply-1", got)
	}
}

func requirePlatformAvailable(t *testing.T, name string) {
	t.Helper()
	if !isPlatformAvailable(name) {
		t.Skipf("platform %q is not compiled into this test build", name)
	}
}

func expectedAvailablePlatformNames() []string {
	names := make([]string, 0, len(platformOrder))
	for _, name := range platformOrder {
		if isPlatformAvailable(name) {
			names = append(names, name)
		}
	}
	return names
}
