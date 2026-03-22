package daemon

import "github.com/bwmarrin/discordgo"

func discordCommandOptions(spec botCommandSpec) []*discordgo.ApplicationCommandOption {
	switch spec.Command {
	case "select":
		return []*discordgo.ApplicationCommandOption{{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "pane",
			Description: "Pane id such as %5 or default:%5",
			Required:    true,
		}}
	case "unmanage":
		return []*discordgo.ApplicationCommandOption{{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "pane",
			Description: "Managed pane id such as %5 or default:%5",
			Required:    true,
		}}
	case "snapshot":
		return []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "lines",
				Description: "Number of lines to capture",
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "mode",
				Description: "Snapshot output mode",
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "image", Value: "image"},
					{Name: "text", Value: "text"},
				},
			},
		}
	case "send":
		return []*discordgo.ApplicationCommandOption{{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "text",
			Description: "Text to send to the current pane",
			Required:    true,
		}}
	case "keys":
		return []*discordgo.ApplicationCommandOption{{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "keys",
			Description: "Tmux keys such as C-c, Enter, or PageUp",
			Required:    true,
		}}
	case "enter":
		return []*discordgo.ApplicationCommandOption{{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "text",
			Description: "Optional text to send before Enter",
		}}
	case "follow":
		return []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "mode",
				Description: "Enable or disable follow mode",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "on", Value: "on"},
					{Name: "off", Value: "off"},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "interval",
				Description: "Optional interval such as 2s",
			},
		}
	default:
		return nil
	}
}
