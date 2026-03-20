package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPathUsesXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg"))
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))

	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}

	want := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), DefaultDirName, "config.toml")
	if path != want {
		t.Fatalf("DefaultPath() = %q, want %q", path, want)
	}
}

func TestDefaultPathFallsBackToHomeConfigDir(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}

	want := filepath.Join(home, ".config", DefaultDirName, "config.toml")
	if path != want {
		t.Fatalf("DefaultPath() = %q, want %q", path, want)
	}
}

func TestLoadMissingDefaultFileIsIgnored(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	loaded, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Exists {
		t.Fatal("Load() Exists = true, want false")
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[daemon]\nunknown = true\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "strict mode") {
		t.Fatalf("Load() error = %q, want strict mode detail", err)
	}
}
