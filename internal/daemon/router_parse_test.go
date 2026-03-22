package daemon

import (
	"strings"
	"testing"
)

func TestParseKeysArgs(t *testing.T) {
	t.Parallel()

	keys, err := parseKeysArgs("ctrl-c enter esc left tab npage f12 ctrl+x m-z s-right c-space m-enter kp9")
	if err != nil {
		t.Fatalf("parseKeysArgs() error = %v", err)
	}
	if strings.Join(keys, " ") != "C-c Enter Escape Left Tab PageDown F12 C-x M-z S-Right C-Space M-Enter KP9" {
		t.Fatalf("keys = %#v, want normalized key sequence", keys)
	}

	if _, err := parseKeysArgs("   "); err == nil {
		t.Fatal("parseKeysArgs(empty) error = nil, want error")
	}
	if _, err := parseKeysArgs("ctrl+1"); err == nil {
		t.Fatal("parseKeysArgs(ctrl+1) error = nil, want error")
	}
}

func TestParseSnapshotArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		value     string
		wantLines int
		wantMode  snapshotMode
		wantErr   bool
	}{
		{name: "default", value: "", wantLines: 120, wantMode: snapshotModeImage},
		{name: "lines only", value: "200", wantLines: 200, wantMode: snapshotModeImage},
		{name: "text only", value: "text", wantLines: 120, wantMode: snapshotModeText},
		{name: "image only", value: "image", wantLines: 120, wantMode: snapshotModeImage},
		{name: "lines then text", value: "200 text", wantLines: 200, wantMode: snapshotModeText},
		{name: "text then lines", value: "text 200", wantLines: 200, wantMode: snapshotModeText},
		{name: "bad mode", value: "plain", wantErr: true},
		{name: "bad lines", value: "0", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines, mode, err := parseSnapshotArgs(tt.value, 120)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseSnapshotArgs(%q) error = nil, want error", tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSnapshotArgs(%q) error = %v", tt.value, err)
			}
			if lines != tt.wantLines {
				t.Fatalf("lines = %d, want %d", lines, tt.wantLines)
			}
			if mode != tt.wantMode {
				t.Fatalf("mode = %q, want %q", mode, tt.wantMode)
			}
		})
	}
}
