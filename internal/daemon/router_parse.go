package daemon

import (
	"strconv"
	"strings"
)

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
