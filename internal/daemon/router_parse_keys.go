package daemon

import (
	"fmt"
	"strings"
)

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
