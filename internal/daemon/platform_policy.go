package daemon

import "strings"

func defaultParseMessage(message IncomingMessage, commandPrefix string) parsedCommand {
	text := strings.TrimSpace(message.Text)
	if text == "" {
		return parsedCommand{}
	}
	if strings.HasPrefix(text, "/") {
		command, args := parseExplicitCommand(text)
		return parsedCommand{Command: command, Args: args}
	}
	if message.IsAppMention {
		command, args := parseExplicitCommand(text)
		return parsedCommand{Command: command, Args: args}
	}
	if prefixed, ok := trimCommandPrefix(text, commandPrefix); ok {
		command, args := parseExplicitCommand(prefixed)
		return parsedCommand{Command: command, Args: args}
	}
	return parsedCommand{Args: text}
}

func defaultPromptText(_ IncomingMessage, spec commandPromptSpec) string {
	return spec.Message
}

func defaultSnapshotMode(mode snapshotMode) snapshotMode {
	return mode
}
