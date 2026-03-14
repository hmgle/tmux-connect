package tmux

import "testing"

func TestParseTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		socket  string
		want    Target
		wantErr bool
	}{
		{name: "pane id", input: "%5", want: Target{Socket: "default", PaneID: "%5"}},
		{name: "pane key", input: "main:%9", want: Target{Socket: "main", PaneID: "%9"}},
		{name: "default socket override", input: "%2", socket: "dev", want: Target{Socket: "dev", PaneID: "%2"}},
		{name: "invalid", input: "5", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseTarget(tt.input, tt.socket)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseTarget() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
