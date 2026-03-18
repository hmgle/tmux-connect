package daemon

import (
	"strings"

	"github.com/portgle/tmux-connect/internal/telegram"
)

type botCommandSpec struct {
	Command     string
	Description string
	Usage       string
}

func daemonCommandSpecs() []botCommandSpec {
	return []botCommandSpec{
		{Command: "start", Description: "Show quick start and commands", Usage: "/start"},
		{Command: "help", Description: "Show command help", Usage: "/help"},
		{Command: "panes", Description: "List managed panes", Usage: "/panes"},
		{Command: "select", Description: "Select the current pane", Usage: "/select <pane>"},
		{Command: "clear", Description: "Clear the current pane", Usage: "/clear"},
		{Command: "unmanage", Description: "Stop managing a pane", Usage: "/unmanage <pane>"},
		{Command: "current", Description: "Show the current pane", Usage: "/current"},
		{Command: "snapshot", Description: "Capture pane output", Usage: "/snapshot [lines] [image|text]"},
		{Command: "send", Description: "Send text to the current pane", Usage: "/send <text>"},
		{Command: "enter", Description: "Send Enter to the current pane", Usage: "/enter"},
		{Command: "ctrlc", Description: "Send Ctrl-C to the current pane", Usage: "/ctrlc"},
		{Command: "follow", Description: "Stream pane updates", Usage: "/follow on [interval]|off"},
	}
}

func telegramMenuCommands() []telegram.BotCommand {
	specs := daemonCommandSpecs()
	commands := make([]telegram.BotCommand, 0, len(specs))
	for _, spec := range specs {
		commands = append(commands, telegram.BotCommand{
			Command:     spec.Command,
			Description: spec.Description,
		})
	}
	return commands
}

func helpText() string {
	lines := make([]string, 0, len(daemonCommandSpecs())+1)
	lines = append(lines, "Commands:")
	for _, spec := range daemonCommandSpecs() {
		lines = append(lines, spec.Usage)
	}
	return strings.Join(lines, "\n")
}
