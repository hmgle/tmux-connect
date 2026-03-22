package daemon

import (
	"fmt"
	"strconv"
	"strings"
)

const followRepeatedPrefixMarker = "...[omitted repeated prefix]\n"

func buildFollowUpdate(previous string, current string) (string, bool) {
	if strings.TrimSpace(current) == "" {
		return "", false
	}
	if previous == "" {
		return strings.TrimSpace(current), true
	}
	if current == previous {
		return "", false
	}
	if strings.HasPrefix(current, previous) {
		delta := current[len(previous):]
		currentView := buildRecentFollowContext(current, 6, 600)
		previousView := buildRecentFollowContext(previous, 6, 600)
		if shouldPreferFollowContextView(delta, previousView, currentView) {
			return currentView, true
		}
		if shouldSendFollowContext(current, delta) {
			currentView = buildRecentFollowContext(current, 4, 240)
			previousView = buildRecentFollowContext(previous, 4, 240)
			if currentView == "" || currentView == previousView {
				return "", false
			}
			return currentView, true
		}
		text := strings.TrimSpace(delta)
		if text == "" {
			return "", false
		}
		return text, true
	}

	if tail, ok := trimRepeatedPrefix(previous, current, 2, 60); ok {
		return followRepeatedPrefixMarker + tail, true
	}

	currentView := buildRecentFollowContext(current, 6, 600)
	previousView := buildRecentFollowContext(previous, 6, 600)
	if currentView == "" || currentView == previousView {
		return "", false
	}
	return currentView, true
}

func shouldPreferFollowContextView(delta string, previousView string, currentView string) bool {
	delta = strings.TrimSpace(delta)
	currentView = strings.TrimSpace(currentView)
	previousView = strings.TrimSpace(previousView)
	if delta == "" || currentView == "" || currentView == previousView {
		return false
	}
	if !strings.Contains(delta, "\n") {
		return false
	}

	deltaLen := len([]rune(delta))
	viewLen := len([]rune(currentView))
	if deltaLen >= 400 && viewLen <= 220 {
		return true
	}
	if viewLen > 0 && deltaLen >= viewLen*3 {
		return true
	}
	return false
}

func shouldSendFollowContext(current string, delta string) bool {
	if !strings.HasSuffix(current, "\n") {
		return true
	}
	trimmed := strings.TrimSpace(delta)
	if trimmed == "" {
		return false
	}
	return !strings.Contains(delta, "\n") && len([]rune(trimmed)) <= 24
}

func buildRecentFollowContext(text string, maxLines int, maxLen int) string {
	lines := effectiveFollowLines(text)
	if len(lines) == 0 {
		return ""
	}

	selected := make([]string, 0, min(len(lines), maxLines))
	used := 0

	for i := len(lines) - 1; i >= 0 && len(selected) < maxLines; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" && used > 0 {
			continue
		}
		lineLen := len([]rune(line))
		added := lineLen
		if len(selected) > 0 {
			added++
		}
		if used > 0 && used+added > maxLen {
			break
		}
		if used == 0 && lineLen > maxLen {
			return truncateLineTail(line, maxLen)
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
	return strings.TrimSpace(strings.Join(selected, "\n"))
}

func effectiveFollowLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return nil
	}

	rawLines := strings.Split(text, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		if idx := strings.LastIndexByte(line, '\r'); idx >= 0 {
			line = line[idx+1:]
		}
		lines = append(lines, line)
	}
	return lines
}

func truncateLineTail(line string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(line)
	if len(runes) <= maxLen {
		return line
	}
	const marker = "... "
	markerRunes := []rune(marker)
	if len(markerRunes) >= maxLen {
		return string(runes[len(runes)-maxLen:])
	}
	tailLen := maxLen - len(markerRunes)
	return marker + string(runes[len(runes)-tailLen:])
}

func debugPreview(text string, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = 120
	}
	quoted := strconv.QuoteToASCII(text)
	runes := []rune(quoted)
	if len(runes) <= maxRunes {
		return quoted
	}
	head := maxRunes / 2
	tail := maxRunes - head - 3
	if tail < 0 {
		tail = 0
	}
	return string(runes[:head]) + "..." + string(runes[len(runes)-tail:])
}

func trimRepeatedPrefix(previous string, current string, minSharedLines int, minSharedRunes int) (string, bool) {
	if strings.HasPrefix(current, previous) {
		tail := strings.TrimLeft(strings.TrimPrefix(current, previous), "\n")
		tail = strings.TrimSpace(tail)
		if tail == "" {
			return "", false
		}
		return tail, true
	}

	prevLines := strings.Split(previous, "\n")
	currLines := strings.Split(current, "\n")
	shared := commonPrefixLines(prevLines, currLines)
	if shared == 0 {
		return "", false
	}
	sharedText := strings.Join(currLines[:shared], "\n")
	if shared < minSharedLines || len([]rune(sharedText)) < minSharedRunes {
		return "", false
	}

	tail := strings.TrimSpace(strings.Join(currLines[shared:], "\n"))
	if tail == "" {
		return "", false
	}
	return tail, true
}

func appendFollowTranscript(current string, chunk string, maxRunes int) string {
	if chunk == "" {
		return current
	}
	current += chunk
	if maxRunes <= 0 {
		return current
	}

	runes := []rune(current)
	if len(runes) <= maxRunes {
		return current
	}

	trimmed := string(runes[len(runes)-maxRunes:])
	if idx := strings.IndexByte(trimmed, '\n'); idx >= 0 && idx+1 < len(trimmed) {
		return trimmed[idx+1:]
	}
	return trimmed
}

func commonPrefixLines(left []string, right []string) int {
	limit := min(len(left), len(right))
	for i := 0; i < limit; i++ {
		if left[i] != right[i] {
			return i
		}
	}
	return limit
}

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
