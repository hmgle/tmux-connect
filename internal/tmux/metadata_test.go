package tmux

import "testing"

func TestMetadataRoundTrip(t *testing.T) {
	t.Parallel()

	meta := BridgeMetadata{
		Managed:          true,
		Mode:             ModeRelay,
		Agent:            AgentCodex,
		Label:            "api",
		CreatedBy:        CreatedByManualAttach,
		LastActivityUnix: 123,
	}

	got := MetadataFromOptions(meta.ToOptions())
	if got.Managed != meta.Managed || got.Mode != meta.Mode || got.Agent != meta.Agent || got.Label != meta.Label || got.CreatedBy != meta.CreatedBy || got.LastActivityUnix != meta.LastActivityUnix {
		t.Fatalf("round trip mismatch: got %#v want %#v", got, meta)
	}
}

func TestParseUserOptions(t *testing.T) {
	t.Parallel()

	output := "@tagb_managed 1\n@tagb_mode relay\nstatus on\n"
	got := parseUserOptions(output)
	if len(got) != 2 {
		t.Fatalf("expected 2 options, got %d", len(got))
	}
	if got[OptionManaged] != "1" || got[OptionMode] != "relay" {
		t.Fatalf("unexpected options: %#v", got)
	}
}
