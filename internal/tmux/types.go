package tmux

import "time"

type Target struct {
	Socket string `json:"socket"`
	PaneID string `json:"pane_id"`
}

func (t Target) PaneKey() string {
	socket := t.Socket
	if socket == "" {
		socket = "default"
	}
	return socket + ":" + t.PaneID
}

func (t Target) Matches(other Target) bool {
	return t.PaneID == other.PaneID && normalizedSocket(t.Socket) == normalizedSocket(other.Socket)
}

type PaneInfo struct {
	Target      Target `json:"target"`
	SessionName string `json:"session_name"`
	WindowID    string `json:"window_id"`
	WindowName  string `json:"window_name"`
	PaneTitle   string `json:"pane_title"`
	CurrentCmd  string `json:"current_cmd"`
	CurrentPath string `json:"current_path"`
	Dead        bool   `json:"dead"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
}

type PaneState struct {
	Info     PaneInfo
	Metadata BridgeMetadata
}

type OutputChunk struct {
	Target     Target
	Text       string
	ReceivedAt time.Time
}
