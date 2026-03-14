package tmux

import (
	"fmt"
	"strings"
)

func ParseTarget(value string, defaultSocket string) (Target, error) {
	ref := strings.TrimSpace(value)
	if ref == "" {
		return Target{}, fmt.Errorf("empty pane reference")
	}
	if strings.HasPrefix(ref, "%") {
		return Target{Socket: normalizedSocket(defaultSocket), PaneID: ref}, nil
	}
	parts := strings.Split(ref, ":")
	if len(parts) == 2 && strings.HasPrefix(parts[1], "%") {
		return Target{Socket: normalizedSocket(parts[0]), PaneID: parts[1]}, nil
	}
	return Target{}, fmt.Errorf("invalid pane reference %q; use %%5 or socket:%%5", value)
}

func normalizedSocket(value string) string {
	if strings.TrimSpace(value) == "" {
		return "default"
	}
	return strings.TrimSpace(value)
}
