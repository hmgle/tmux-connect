package daemon

import (
	"fmt"
	"strings"
)

func formatFollowMessage(paneKey string, text string, maxLen int) string {
	text = strings.TrimSpace(text)
	if maxLen > 0 {
		text = truncateForTelegram(text, maxLen)
	}
	if text == "" {
		return fmt.Sprintf("[%s] (empty output)", paneKey)
	}
	return fmt.Sprintf("[%s]\n%s", paneKey, text)
}

func truncateForTelegram(text string, maxLen int) string {
	if maxLen <= 0 {
		return text
	}

	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}

	const marker = "...[truncated]\n"
	markerRunes := []rune(marker)
	if len(markerRunes) >= maxLen {
		return string(runes[len(runes)-maxLen:])
	}

	tailLen := maxLen - len(markerRunes)
	if tail := trailingLinesWithinLimit(text, tailLen); tail != "" {
		return marker + tail
	}

	tail := string(runes[len(runes)-tailLen:])
	return marker + tail
}

func trailingLinesWithinLimit(text string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}

	lines := strings.Split(text, "\n")
	selected := make([]string, 0, len(lines))
	used := 0

	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		lineLen := len([]rune(line))
		added := lineLen
		if len(selected) > 0 {
			added++
		}
		if added > maxLen-used {
			break
		}
		selected = append(selected, line)
		used += added
	}

	if len(selected) == 0 {
		return ""
	}

	for left, right := 0, len(selected)-1; left < right; left, right = left+1, right-1 {
		selected[left], selected[right] = selected[right], selected[left]
	}
	return strings.Join(selected, "\n")
}
