package daemon

import "strings"

type botCommandSpec struct {
	Command     string
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
		{Command: "start", Description: "Show quick start and commands", Usage: "/start"},
		{Command: "help", Description: "Show command help", Usage: "/help"},
		{Command: "panes", Description: "List managed panes", Usage: "/panes"},
		{Command: "select", Description: "Select the current pane", Usage: "/select <pane>", Prompt: &commandPromptSpec{
			Message:     "Reply with the pane to select, for example %5 or default:%5.",
			Placeholder: "%5",
		}},
		{Command: "clear", Description: "Clear the current pane", Usage: "/clear"},
		{Command: "unmanage", Description: "Stop managing a pane", Usage: "/unmanage <pane>", Prompt: &commandPromptSpec{
			Message:     "Reply with the pane to stop managing, for example %5 or default:%5.",
			Placeholder: "%5",
		}},
		{Command: "current", Description: "Show the current pane", Usage: "/current"},
		{Command: "snapshot", Description: "Capture pane output", Usage: "/snapshot [lines] [image|text]"},
		{Command: "send", Description: "Send text to the current pane", Usage: "/send <text>", Prompt: &commandPromptSpec{
			Message:     "Reply with the text to send to the current pane.",
			Placeholder: "status",
		}},
		{Command: "enter", Description: "Send Enter to the current pane", Usage: "/enter"},
		{Command: "ctrlc", Description: "Send Ctrl-C to the current pane", Usage: "/ctrlc"},
		{Command: "follow", Description: "Stream pane updates", Usage: "/follow on [interval]|off", Prompt: &commandPromptSpec{
			Message:     "Reply with follow mode: on, on 2s, or off.",
			Placeholder: "on 2s",
		}},
	}
}

func helpText() string {
	lines := make([]string, 0, len(daemonCommandSpecs())+1)
	lines = append(lines, "Commands:")
	for _, spec := range daemonCommandSpecs() {
		lines = append(lines, spec.Usage)
	}
	return strings.Join(lines, "\n")
}

func findCommandSpec(command string) (botCommandSpec, bool) {
	command = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(command)), "/")
	for _, spec := range daemonCommandSpecs() {
		if spec.Command == command {
			return spec, true
		}
	}
	return botCommandSpec{}, false
}
