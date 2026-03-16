package tmux

import "testing"

func TestPaneKeyAndMatches(t *testing.T) {
	t.Parallel()

	target := Target{Socket: "", PaneID: "%5"}
	if got, want := target.PaneKey(), "default:%5"; got != want {
		t.Fatalf("PaneKey() = %q, want %q", got, want)
	}
	if !target.Matches(Target{Socket: "default", PaneID: "%5"}) {
		t.Fatal("expected sockets to normalize to default")
	}
	if target.Matches(Target{Socket: "main", PaneID: "%5"}) {
		t.Fatal("did not expect different sockets to match")
	}
}

func TestSnapshotDiff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		previous string
		current  string
		want     string
	}{
		{
			name:     "unchanged",
			previous: "abc",
			current:  "abc",
			want:     "",
		},
		{
			name:     "new lines",
			previous: "one\ntwo",
			current:  "one\ntwo\nthree",
			want:     "three",
		},
		{
			name:     "screen refresh",
			previous: "one\ntwo",
			current:  "alpha\nbeta",
			want:     "alpha\nbeta",
		},
		{
			name:     "scrolls forward by one line",
			previous: "one\ntwo\nthree",
			current:  "two\nthree\nfour",
			want:     "four",
		},
		{
			name:     "inline prompt grows on same line",
			previous: ">>> ",
			current:  ">>> 300",
			want:     "300",
		},
		{
			name:     "prompt grows then emits result and next prompt",
			previous: ">>> 300 + 123",
			current:  ">>> 300 + 123\n423\n>>> ",
			want:     "423\n>>> ",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := snapshotDiff(tc.previous, tc.current)
			if got != tc.want {
				t.Fatalf("snapshotDiff(%q, %q) = %q, want %q", tc.previous, tc.current, got, tc.want)
			}
		})
	}
}
