package tagb

import (
	"fmt"
	"strings"
)

const DefaultSocket = "default"

type Target struct {
	Socket string `json:"socket"`
	PaneID string `json:"pane_id"`
}

func ParseTarget(input, defaultSocket string) (Target, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return Target{}, fmt.Errorf("pane target cannot be empty")
	}

	target := Target{}
	if strings.Contains(input, ":") {
		parts := strings.SplitN(input, ":", 2)
		target.Socket = strings.TrimSpace(parts[0])
		target.PaneID = strings.TrimSpace(parts[1])
	} else {
		target.Socket = strings.TrimSpace(defaultSocket)
		target.PaneID = input
	}

	if target.Socket == "" {
		target.Socket = DefaultSocket
	}
	if !strings.HasPrefix(target.PaneID, "%") {
		return Target{}, fmt.Errorf("pane %q must start with %%", target.PaneID)
	}
	return target, nil
}

func (t Target) NormalizedSocket() string {
	if strings.TrimSpace(t.Socket) == "" {
		return DefaultSocket
	}
	return t.Socket
}

func (t Target) PaneKey() string {
	return t.NormalizedSocket() + ":" + t.PaneID
}
