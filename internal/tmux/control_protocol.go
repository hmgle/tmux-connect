package tmux

import (
	"fmt"
	"strings"
	"time"
)

func parseNotification(target Target, line string) (OutputChunk, bool, error) {
	if strings.HasPrefix(line, "%output ") {
		parts := strings.SplitN(line, " ", 3)
		if len(parts) != 3 {
			return OutputChunk{}, false, fmt.Errorf("malformed %%output notification: %q", line)
		}
		if parts[1] != target.PaneID {
			return OutputChunk{}, false, nil
		}
		text, err := decodeTmuxEscapes(parts[2])
		if err != nil {
			return OutputChunk{}, false, err
		}
		return OutputChunk{Target: target, Text: text, ReceivedAt: time.Now()}, true, nil
	}
	if strings.HasPrefix(line, "%extended-output ") {
		parts := strings.SplitN(line, " ", 4)
		if len(parts) < 4 {
			return OutputChunk{}, false, fmt.Errorf("malformed %%extended-output notification: %q", line)
		}
		if parts[1] != target.PaneID {
			return OutputChunk{}, false, nil
		}
		index := strings.Index(line, ": ")
		if index == -1 {
			return OutputChunk{}, false, fmt.Errorf("missing payload in %%extended-output notification: %q", line)
		}
		text, err := decodeTmuxEscapes(line[index+2:])
		if err != nil {
			return OutputChunk{}, false, err
		}
		return OutputChunk{Target: target, Text: text, ReceivedAt: time.Now()}, true, nil
	}
	return OutputChunk{}, false, nil
}

func cleanControlLine(line string) string {
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	line = strings.ReplaceAll(line, "\x1bP1000p", "")
	line = strings.ReplaceAll(line, "\x1b\\", "")
	return strings.TrimSpace(line)
}

func decodeTmuxEscapes(value string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if ch != '\\' {
			b.WriteByte(ch)
			continue
		}
		if i+3 >= len(value) {
			return "", fmt.Errorf("invalid tmux escape")
		}
		octal := value[i+1 : i+4]
		var decoded byte
		for j := 0; j < 3; j++ {
			if octal[j] < '0' || octal[j] > '7' {
				return "", fmt.Errorf("invalid tmux escape %q", octal)
			}
			decoded = decoded*8 + (octal[j] - '0')
		}
		b.WriteByte(decoded)
		i += 3
	}
	return b.String(), nil
}
