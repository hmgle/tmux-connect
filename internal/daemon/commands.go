package daemon

import (
	"fmt"
	"strings"
)

const defaultSlackCommandPrefix = "tmux:"

type botCommandSpec struct {
	Command     string
	Aliases     []string
	Description string
	Usage       string
	Prompt      *commandPromptSpec
}

type commandPromptSpec struct {
	Message     string
	Placeholder string
}

func daemonCommandSpecs() []botCommandSpec {
	return []botCommandSpec{
		{Command: "start", Description: "Show quick start and commands", Usage: "start"},
		{Command: "help", Description: "Show command help", Usage: "help"},
		{Command: "panes", Description: "List managed panes", Usage: "panes"},
		{Command: "select", Description: "Select the current pane", Usage: "select <pane>", Prompt: &commandPromptSpec{
			Message:     "Reply with the pane to select, for example %5 or default:%5.",
			Placeholder: "%5",
		}},
		{Command: "clear", Description: "Clear the current pane", Usage: "clear"},
		{Command: "unmanage", Description: "Stop managing a pane", Usage: "unmanage <pane>", Prompt: &commandPromptSpec{
			Message:     "Reply with the pane to stop managing, for example %5 or default:%5.",
			Placeholder: "%5",
		}},
		{Command: "current", Description: "Show the current pane", Usage: "current"},
		{Command: "snapshot", Description: "Capture pane output", Usage: "snapshot [lines] [image|text]"},
		{Command: "send", Description: "Send text to the current pane (legacy explicit form)", Usage: "send <text>", Prompt: &commandPromptSpec{
			Message:     "Reply with the text to send to the current pane.",
			Placeholder: "status",
		}},
		{Command: "keys", Aliases: []string{"key"}, Description: "Send tmux keys like Enter or C-c", Usage: "keys <key...>", Prompt: &commandPromptSpec{
			Message:     "Reply with the tmux key names to send, for example C-c or Enter.",
			Placeholder: "C-c",
		}},
		{Command: "enter", Description: "Send Enter, or send text and Enter", Usage: "enter [text]"},
		{Command: "ctrlc", Aliases: []string{"ctrl-c"}, Description: "Send Ctrl-C to the current pane", Usage: "ctrlc"},
		{Command: "follow", Description: "Stream pane updates", Usage: "follow on [interval]|off", Prompt: &commandPromptSpec{
			Message:     "Reply with follow mode: on, on 2s, or off.",
			Placeholder: "on 2s",
		}},
	}
}

func helpText(commandPrefix string) string {
	lines := make([]string, 0, len(daemonCommandSpecs())+4)
	lines = append(lines, "Commands:")
	if commandPrefix != "" {
		lines = append(lines, fmt.Sprintf(`In channels, mention the bot with a command such as "@bot panes". In DMs and managed threads, prefix commands with %q, for example %q.`, commandPrefix, commandPrefix+" panes"))
		lines = append(lines, fmt.Sprintf("In DMs and managed threads, plain text sends to the current pane. Use %q to execute and %q for control keys.", formatCommandUsage(commandPrefix, "enter"), formatCommandUsage(commandPrefix, "keys C-c")))
		lines = append(lines, fmt.Sprintf("Use %q when the text itself starts with \"/\".", formatCommandUsage(commandPrefix, "send <text>")))
	} else {
		lines = append(lines, fmt.Sprintf("Plain text sends to the current pane. Use %q to execute and %q for control keys.", formatCommandUsage(commandPrefix, "enter"), formatCommandUsage(commandPrefix, "keys C-c")))
		lines = append(lines, fmt.Sprintf("Use %q when the text itself starts with \"/\".", formatCommandUsage(commandPrefix, "send <text>")))
	}
	for _, spec := range daemonCommandSpecs() {
		lines = append(lines, formatCommandUsage(commandPrefix, spec.Usage))
	}
	return strings.Join(lines, "\n")
}

func findCommandSpec(command string) (botCommandSpec, bool) {
	command = normalizeCommandName(command)
	for _, spec := range daemonCommandSpecs() {
		if spec.Command == command {
			return spec, true
		}
		for _, alias := range spec.Aliases {
			if alias == command {
				return spec, true
			}
		}
	}
	return botCommandSpec{}, false
}

func normalizeCommandName(command string) string {
	command = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(command)), "/")
	if mention := strings.Index(command, "@"); mention >= 0 {
		command = command[:mention]
	}
	return command
}

func formatCommandUsage(commandPrefix string, usage string) string {
	usage = strings.TrimSpace(usage)
	if usage == "" {
		return ""
	}
	if commandPrefix != "" {
		return commandPrefix + " " + usage
	}
	return "/" + usage
}
