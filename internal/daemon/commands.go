package daemon

import (
	"fmt"
	"strings"
)

const defaultSlackCommandPrefix = "tmux:"
const defaultDiscordCommandPrefix = "tmux:"

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
	return platformHelpText("telegram", commandPrefix)
}

func snapshotCommandUsage(platform string) string {
	if isWeixinPlatform(platform) {
		return "snapshot [lines] [text]"
	}
	return "snapshot [lines] [image|text]"
}

func commandUsageForPlatform(platform string, spec botCommandSpec) string {
	if spec.Command == "snapshot" {
		return snapshotCommandUsage(platform)
	}
	return spec.Usage
}

func appendDefaultPlainTextHelp(lines []string, commandPrefix string) []string {
	lines = append(lines, fmt.Sprintf("Plain text targets the current pane and may execute immediately when execute mode is enabled. Use %q for raw text, %q to execute, and %q for control keys.", formatCommandUsage(commandPrefix, "send <text>"), formatCommandUsage(commandPrefix, "enter"), formatCommandUsage(commandPrefix, "keys C-c")))
	lines = append(lines, fmt.Sprintf("Use %q when the text itself starts with \"/\".", formatCommandUsage(commandPrefix, "send <text>")))
	return lines
}

func platformHelpText(platform string, commandPrefix string) string {
	platform = strings.TrimSpace(strings.ToLower(platform))
	lines := make([]string, 0, len(daemonCommandSpecs())+4)
	lines = append(lines, "Commands:")
	switch {
	case platform == "slack" && commandPrefix != "":
		lines = append(lines, fmt.Sprintf(`In channels, mention the bot with a command such as "@bot panes". In DMs and managed threads, prefix commands with %q, for example %q.`, commandPrefix, commandPrefix+" panes"))
		lines = append(lines, fmt.Sprintf("In DMs and managed threads, plain text targets the current pane and may execute immediately when execute mode is enabled. Use %q for raw text, %q to execute, and %q for control keys.", formatCommandUsage(commandPrefix, "send <text>"), formatCommandUsage(commandPrefix, "enter"), formatCommandUsage(commandPrefix, "keys C-c")))
		lines = append(lines, fmt.Sprintf("Use %q when the text itself starts with \"/\".", formatCommandUsage(commandPrefix, "send <text>")))
	case platform == "discord":
		prefix := strings.TrimSpace(commandPrefix)
		if prefix == "" {
			prefix = defaultDiscordCommandPrefix
		}
		lines = append(lines, `Use slash commands such as "/panes" for explicit actions.`)
		lines = append(lines, fmt.Sprintf(`In channels, you can also prefix text commands with %q, for example %q.`, prefix, prefix+" panes"))
		lines = append(lines, fmt.Sprintf(`In DMs, plain text targets the current pane and may execute immediately when execute mode is enabled. Use %q for raw text or %q to force a command.`, "/send <text>", prefix+" current"))
		lines = append(lines, `When the text itself starts with "/", use "/send <text>" to pass it through.`)
	case platform == "whatsapp":
		lines = append(lines, `In WhatsApp private chats, plain text targets the current pane and may execute immediately when execute mode is enabled.`)
		lines = append(lines, `Use slash commands such as "/panes" or "/follow on" for explicit actions, and use "/send <text>" when the text itself starts with "/".`)
		lines = append(lines, `If WhatsApp self-chat mode is enabled, plain text is disabled to avoid reply loops, so use explicit slash commands such as "/send <text>" or "/enter <text>".`)
		lines = append(lines, `When the bot asks for more input, reply in the same chat. For pane selection prompts, replying with "1" or "2" is supported.`)
	case platform == "feishu":
		lines = append(lines, `In Feishu private chats, plain text targets the current pane and may execute immediately when execute mode is enabled.`)
		lines = append(lines, `In Feishu groups, mention the bot with a command such as "@bot panes". Plain text without @bot is ignored.`)
		lines = append(lines, `Use "/send <text>" when the text itself starts with "/". Static cards are used for help and pane selection prompts.`)
	case platform == "weixin":
		lines = appendDefaultPlainTextHelp(lines, commandPrefix)
		lines = append(lines, `Weixin iLink currently forces "/snapshot" to text output because image replies render as placeholder boxes in the client.`)
	default:
		lines = appendDefaultPlainTextHelp(lines, commandPrefix)
	}
	usagePrefix := commandPrefix
	if platform == "discord" {
		usagePrefix = ""
	}
	for _, spec := range daemonCommandSpecs() {
		lines = append(lines, formatCommandUsage(usagePrefix, commandUsageForPlatform(platform, spec)))
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
