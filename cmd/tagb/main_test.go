package main

import (
	"bytes"
	"testing"

	tagbconfig "github.com/hmgle/tmux-connect/internal/config"
)

func TestParseGlobalArgsUsesConfigSocket(t *testing.T) {
	t.Parallel()

	socket, args, err := parseGlobalArgs([]string{"list"}, tagbconfig.File{
		Tmux: tagbconfig.Tmux{Socket: stringPtr("cfg-sock")},
	})
	if err != nil {
		t.Fatalf("parseGlobalArgs() error = %v", err)
	}
	if socket != "cfg-sock" {
		t.Fatalf("socket = %q, want cfg-sock", socket)
	}
	if len(args) != 1 || args[0] != "list" {
		t.Fatalf("args = %#v, want [list]", args)
	}
}

func TestParseGlobalArgsEnvOverridesConfigSocket(t *testing.T) {
	t.Setenv("TAGB_TMUX_SOCKET", "env-sock")

	socket, _, err := parseGlobalArgs([]string{"list"}, tagbconfig.File{
		Tmux: tagbconfig.Tmux{Socket: stringPtr("cfg-sock")},
	})
	if err != nil {
		t.Fatalf("parseGlobalArgs() error = %v", err)
	}
	if socket != "env-sock" {
		t.Fatalf("socket = %q, want env-sock", socket)
	}
}

func TestParseGlobalArgsFlagOverridesEnvSocket(t *testing.T) {
	t.Setenv("TAGB_TMUX_SOCKET", "env-sock")

	socket, _, err := parseGlobalArgs([]string{"--socket", "flag-sock", "list"}, tagbconfig.File{
		Tmux: tagbconfig.Tmux{Socket: stringPtr("cfg-sock")},
	})
	if err != nil {
		t.Fatalf("parseGlobalArgs() error = %v", err)
	}
	if socket != "flag-sock" {
		t.Fatalf("socket = %q, want flag-sock", socket)
	}
}

func TestParseServeArgsUsesConfigListen(t *testing.T) {
	t.Parallel()

	listen, err := parseServeArgs(&bytes.Buffer{}, tagbconfig.Serve{Listen: stringPtr("127.0.0.1:9090")}, nil)
	if err != nil {
		t.Fatalf("parseServeArgs() error = %v", err)
	}
	if listen != "127.0.0.1:9090" {
		t.Fatalf("listen = %q, want 127.0.0.1:9090", listen)
	}
}

func TestParseServeArgsFlagOverridesConfigListen(t *testing.T) {
	t.Parallel()

	listen, err := parseServeArgs(&bytes.Buffer{}, tagbconfig.Serve{Listen: stringPtr("127.0.0.1:9090")}, []string{"--listen", "127.0.0.1:9191"})
	if err != nil {
		t.Fatalf("parseServeArgs() error = %v", err)
	}
	if listen != "127.0.0.1:9191" {
		t.Fatalf("listen = %q, want 127.0.0.1:9191", listen)
	}
}

func stringPtr(value string) *string {
	return &value
}
