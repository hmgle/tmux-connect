package daemon

import (
	"bytes"
	"flag"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAvailablePlatformNames(t *testing.T) {
	t.Parallel()

	got := availablePlatformNames()
	want := expectedAvailablePlatformNames()
	if !slices.Equal(got, want) {
		t.Fatalf("availablePlatformNames() = %q, want %q", got, want)
	}
}

func TestPrintUsageIncludesCompiledPlatforms(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	printUsage(&buf)
	output := buf.String()
	platformSummary := availablePlatformSummary()
	if platformSummary == "" {
		platformSummary = "(none)"
	}
	defaultPlatform := defaultPlatformName()
	if defaultPlatform == "" {
		defaultPlatform = "(none)"
	}

	for _, want := range []string{
		"Compiled platforms:",
		platformSummary,
		"Default platform:",
		"\n  " + defaultPlatform + "\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("printUsage() output = %q, want %q", output, want)
		}
	}
}

func TestPlatformFlagUsageIncludesCompiledPlatforms(t *testing.T) {
	t.Parallel()

	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	cfg := Config{}
	bindConfigFlags(fs, &cfg, daemonConfigDefaults{
		platform: defaultPlatformName(),
		dbPath:   filepath.Join(t.TempDir(), "tmuxconn.db"),
	})

	platformFlag := fs.Lookup("platform")
	if platformFlag == nil {
		t.Fatal("platform flag = nil")
	}
	if !strings.Contains(platformFlag.Usage, availablePlatformChoices()) {
		t.Fatalf("platform flag usage = %q", platformFlag.Usage)
	}
}

func TestParseConfigRejectsUnknownPlatformWithCompiledPlatformList(t *testing.T) {
	t.Parallel()

	_, err := parseConfig([]string{
		"--platform", "matrix",
		"--db", filepath.Join(t.TempDir(), "tmuxconn.db"),
	}, &bytes.Buffer{}, false)
	if err == nil {
		t.Fatal("parseConfig() error = nil, want error")
	}
	for _, want := range []string{
		`unsupported --platform "matrix"`,
		"compiled platforms: " + availablePlatformSummary(),
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("parseConfig() error = %q, want %q", err, want)
		}
	}
}
