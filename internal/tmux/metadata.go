package tmux

import (
	"strconv"
	"strings"
	"time"
)

type Mode string

const (
	ModeRelay Mode = "relay"
)

type Agent string

const (
	AgentUnknown Agent = "unknown"
	AgentClaude  Agent = "claude"
	AgentCodex   Agent = "codex"
	AgentGemini  Agent = "gemini"
	AgentCursor  Agent = "cursor"
)

const (
	CreatedByManualAttach = "manual-attach"
)

const (
	OptionManaged      = "@tagb_managed"
	OptionMode         = "@tagb_mode"
	OptionAgent        = "@tagb_agent"
	OptionLabel        = "@tagb_label"
	OptionCreatedBy    = "@tagb_created_by"
	OptionLastActivity = "@tagb_last_activity_unix"
)

type BridgeMetadata struct {
	Managed          bool   `json:"managed"`
	Mode             Mode   `json:"mode"`
	Agent            Agent  `json:"agent"`
	Label            string `json:"label"`
	CreatedBy        string `json:"created_by"`
	LastActivityUnix int64  `json:"last_activity_unix"`
}

func DefaultMetadata() BridgeMetadata {
	return BridgeMetadata{
		Managed: false,
		Mode:    ModeRelay,
		Agent:   AgentUnknown,
	}
}

func MetadataFromOptions(opts map[string]string) BridgeMetadata {
	meta := DefaultMetadata()
	if opts[OptionManaged] == "1" {
		meta.Managed = true
	}
	if mode := strings.TrimSpace(opts[OptionMode]); mode != "" {
		meta.Mode = Mode(mode)
	}
	if agent := strings.TrimSpace(opts[OptionAgent]); agent != "" {
		meta.Agent = NormalizeAgent(agent)
	}
	meta.Label = strings.TrimSpace(opts[OptionLabel])
	meta.CreatedBy = strings.TrimSpace(opts[OptionCreatedBy])
	if raw := strings.TrimSpace(opts[OptionLastActivity]); raw != "" {
		if unix, err := strconv.ParseInt(raw, 10, 64); err == nil {
			meta.LastActivityUnix = unix
		}
	}
	return meta
}

func (m BridgeMetadata) ToOptions() map[string]string {
	return map[string]string{
		OptionManaged:      boolString(m.Managed),
		OptionMode:         string(defaultMode(m.Mode)),
		OptionAgent:        string(NormalizeAgent(string(m.Agent))),
		OptionLabel:        strings.TrimSpace(m.Label),
		OptionCreatedBy:    strings.TrimSpace(m.CreatedBy),
		OptionLastActivity: strconv.FormatInt(defaultLastActivity(m.LastActivityUnix), 10),
	}
}

func NormalizeAgent(value string) Agent {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "unknown":
		return AgentUnknown
	case "claude":
		return AgentClaude
	case "codex":
		return AgentCodex
	case "gemini":
		return AgentGemini
	case "cursor":
		return AgentCursor
	default:
		return AgentUnknown
	}
}

func defaultMode(mode Mode) Mode {
	if mode == "" {
		return ModeRelay
	}
	return mode
}

func defaultLastActivity(unix int64) int64 {
	if unix <= 0 {
		return time.Now().Unix()
	}
	return unix
}

func boolString(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
