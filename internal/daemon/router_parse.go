package daemon

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func (r *Router) parseCommand(message IncomingMessage, text string) (string, string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ""
	}
	if strings.HasPrefix(text, "/") {
		return parseExplicitCommand(text)
	}
	if strings.EqualFold(strings.TrimSpace(message.Chat.Platform), "slack") {
		if message.IsAppMention {
			return parseExplicitCommand(text)
		}
		if prefixed, ok := trimCommandPrefix(text, r.slackCommandPrefix); ok {
			return parseExplicitCommand(prefixed)
		}
		return "", text
	}
	return "", text
}

func parseExplicitCommand(text string) (string, string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "help", ""
	}
	command := text
	args := ""
	if idx := strings.IndexAny(text, " \n\t"); idx >= 0 {
		command = text[:idx]
		args = strings.TrimSpace(text[idx+1:])
	}
	command = normalizeCommandName(command)
	if spec, ok := findCommandSpec(command); ok {
		return spec.Command, args
	}
	return command, args
}

func trimCommandPrefix(text string, prefix string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", false
	}
	head := text
	rest := ""
	if idx := strings.IndexAny(text, " \n\t"); idx >= 0 {
		head = text[:idx]
		rest = strings.TrimSpace(text[idx+1:])
	}
	if !strings.EqualFold(head, prefix) {
		return "", false
	}
	return rest, true
}

func optionalInt(value string, fallback int) (int, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func parseKeysArgs(value string) ([]string, error) {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return nil, fmt.Errorf("missing keys")
	}
	keys := make([]string, 0, len(fields))
	for _, field := range fields {
		key, ok := normalizeTmuxKeyName(field)
		if !ok {
			return nil, fmt.Errorf("%q is not a recognized tmux key name", field)
		}
		keys = append(keys, key)
	}
	return keys, nil
}

var namedTmuxKeys = map[string]string{
	"enter":     "Enter",
	"return":    "Enter",
	"esc":       "Escape",
	"escape":    "Escape",
	"tab":       "Tab",
	"btab":      "BTab",
	"space":     "Space",
	"bspace":    "BSpace",
	"backspace": "BSpace",
	"up":        "Up",
	"down":      "Down",
	"left":      "Left",
	"right":     "Right",
	"home":      "Home",
	"end":       "End",
	"pageup":    "PageUp",
	"pgup":      "PageUp",
	"ppage":     "PageUp",
	"pagedown":  "PageDown",
	"pgdn":      "PageDown",
	"npage":     "PageDown",
	"insert":    "Insert",
	"ic":        "IC",
	"delete":    "Delete",
	"del":       "Delete",
	"dc":        "DC",
	"ctrlc":     "C-c",
	"f1":        "F1",
	"f2":        "F2",
	"f3":        "F3",
	"f4":        "F4",
	"f5":        "F5",
	"f6":        "F6",
	"f7":        "F7",
	"f8":        "F8",
	"f9":        "F9",
	"f10":       "F10",
	"f11":       "F11",
	"f12":       "F12",
	"kp0":       "KP0",
	"kp1":       "KP1",
	"kp2":       "KP2",
	"kp3":       "KP3",
	"kp4":       "KP4",
	"kp5":       "KP5",
	"kp6":       "KP6",
	"kp7":       "KP7",
	"kp8":       "KP8",
	"kp9":       "KP9",
}

func normalizeTmuxKeyName(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	if mapped, ok := namedTmuxKeys[lower]; ok {
		return mapped, true
	}
	for _, candidate := range []struct {
		prefixes []string
		modifier string
	}{
		{prefixes: []string{"ctrl-", "ctrl+", "c-"}, modifier: "C"},
		{prefixes: []string{"m-"}, modifier: "M"},
		{prefixes: []string{"s-"}, modifier: "S"},
	} {
		for _, prefix := range candidate.prefixes {
			if strings.HasPrefix(lower, prefix) {
				remainder := trimmed[len(prefix):]
				return normalizeModifiedTmuxKeyName(candidate.modifier, remainder)
			}
		}
	}
	return "", false
}

func normalizeModifiedTmuxKeyName(modifier string, remainder string) (string, bool) {
	trimmed := strings.TrimSpace(remainder)
	lower := strings.ToLower(trimmed)
	if len(lower) == 1 && lower[0] >= 'a' && lower[0] <= 'z' {
		return modifier + "-" + lower, true
	}
	if mapped, ok := namedTmuxKeys[lower]; ok {
		return modifier + "-" + mapped, true
	}
	return "", false
}

func keysUsage(commandPrefix string) string {
	return fmt.Sprintf(
		"usage: %s\n\nExamples: C-c, Enter, Tab, Escape, Up, PageUp, F1-F12, KP0-KP9\nModifiers: C-a..C-z, ctrl-c, ctrl+x, C-Space, M-x, M-Enter, S-Left",
		formatCommandUsage(commandPrefix, "keys <key...>"),
	)
}

func parseFollowArgs(value string) (string, FollowOptions, error) {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return "", FollowOptions{}, fmt.Errorf("missing follow mode")
	}

	mode := strings.ToLower(fields[0])
	switch mode {
	case "off":
		if len(fields) != 1 {
			return "", FollowOptions{}, fmt.Errorf("unexpected follow args")
		}
		return mode, FollowOptions{}, nil
	case "on":
		if len(fields) == 1 {
			return mode, FollowOptions{}, nil
		}
		if len(fields) != 2 {
			return "", FollowOptions{}, fmt.Errorf("unexpected follow args")
		}
		interval, err := parsePositiveDuration(fields[1])
		if err != nil {
			return "", FollowOptions{}, err
		}
		return mode, FollowOptions{MinInterval: interval}, nil
	default:
		return "", FollowOptions{}, fmt.Errorf("unknown follow mode")
	}
}

func parsePositiveDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("duration is required")
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0, fmt.Errorf("duration must be > 0")
		}
		return time.Duration(seconds) * time.Second, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("duration must be > 0")
	}
	return parsed, nil
}

type snapshotMode string

const (
	snapshotModeImage snapshotMode = "image"
	snapshotModeText  snapshotMode = "text"
)

func parseSnapshotArgs(value string, fallbackLines int) (int, snapshotMode, error) {
	lines := fallbackLines
	mode := snapshotModeImage
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return lines, mode, nil
	}

	for _, field := range fields {
		switch strings.ToLower(strings.TrimSpace(field)) {
		case "":
			continue
		case string(snapshotModeImage):
			mode = snapshotModeImage
		case string(snapshotModeText):
			mode = snapshotModeText
		default:
			parsed, err := strconv.Atoi(field)
			if err != nil {
				return 0, "", fmt.Errorf("invalid snapshot arg %q", field)
			}
			lines = parsed
		}
	}

	if lines <= 0 {
		return 0, "", fmt.Errorf("snapshot lines must be > 0")
	}
	return lines, mode, nil
}
