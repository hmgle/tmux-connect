package daemon

import "strings"

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
