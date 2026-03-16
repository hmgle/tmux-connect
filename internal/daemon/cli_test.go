package daemon

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font/gofont/gomono"
)

func TestParseConfigAcceptsSnapshotRenderFlags(t *testing.T) {
	t.Parallel()

	fontPath := writeTempFont(t, "mono.ttf")
	cfg, err := parseConfig([]string{
		"--telegram-token", "token",
		"--db", filepath.Join(t.TempDir(), "tagb.db"),
		"--telegram-snapshot-theme", "light",
		"--telegram-snapshot-font-size", "18",
		"--telegram-snapshot-font-file", fontPath,
	}, &bytes.Buffer{}, true)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.SnapshotTheme != "light" {
		t.Fatalf("snapshot theme = %q, want light", cfg.SnapshotTheme)
	}
	if cfg.SnapshotFontSize != 18 {
		t.Fatalf("snapshot font size = %v, want 18", cfg.SnapshotFontSize)
	}
	if cfg.SnapshotFontFile != fontPath {
		t.Fatalf("snapshot font file = %q, want %q", cfg.SnapshotFontFile, fontPath)
	}
}

func TestParseConfigReadsSnapshotRenderEnv(t *testing.T) {
	fontPath := writeTempFont(t, "env-font.ttf")
	t.Setenv("TAGB_TELEGRAM_TOKEN", "token")
	t.Setenv("TAGB_TELEGRAM_SNAPSHOT_THEME", "light")
	t.Setenv("TAGB_TELEGRAM_SNAPSHOT_FONT_SIZE", "16.5")
	t.Setenv("TAGB_TELEGRAM_SNAPSHOT_FONT_FILE", fontPath)

	cfg, err := parseConfig([]string{"--db", filepath.Join(t.TempDir(), "tagb.db")}, &bytes.Buffer{}, true)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.SnapshotTheme != "light" {
		t.Fatalf("snapshot theme = %q, want light", cfg.SnapshotTheme)
	}
	if cfg.SnapshotFontSize != 16.5 {
		t.Fatalf("snapshot font size = %v, want 16.5", cfg.SnapshotFontSize)
	}
	if cfg.SnapshotFontFile != fontPath {
		t.Fatalf("snapshot font file = %q, want %q", cfg.SnapshotFontFile, fontPath)
	}
}

func TestParseConfigRejectsInvalidSnapshotTheme(t *testing.T) {
	t.Parallel()

	_, err := parseConfig([]string{
		"--telegram-token", "token",
		"--db", filepath.Join(t.TempDir(), "tagb.db"),
		"--telegram-snapshot-theme", "sepia",
	}, &bytes.Buffer{}, true)
	if err == nil {
		t.Fatal("parseConfig() error = nil, want error")
	}
}

func TestParseConfigRejectsInvalidSnapshotFontSizeEnv(t *testing.T) {
	t.Setenv("TAGB_TELEGRAM_SNAPSHOT_FONT_SIZE", "large")
	_, err := parseConfig([]string{
		"--telegram-token", "token",
		"--db", filepath.Join(t.TempDir(), "tagb.db"),
	}, &bytes.Buffer{}, true)
	if err == nil {
		t.Fatal("parseConfig() error = nil, want error")
	}
}

func TestParseConfigRejectsInvalidSnapshotFontExtension(t *testing.T) {
	t.Parallel()

	fontPath := filepath.Join(t.TempDir(), "mono.txt")
	if err := os.WriteFile(fontPath, []byte("not a font"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	_, err := parseConfig([]string{
		"--telegram-token", "token",
		"--db", filepath.Join(t.TempDir(), "tagb.db"),
		"--telegram-snapshot-font-file", fontPath,
	}, &bytes.Buffer{}, true)
	if err == nil {
		t.Fatal("parseConfig() error = nil, want error")
	}
}

func writeTempFont(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, gomono.TTF, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
