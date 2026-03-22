package tmuxconn

import (
	"flag"
	"strings"
)

type commandFlags struct {
	fs      *flag.FlagSet
	jsonOut *bool
}

func (a *App) newCommandFlags(name string) commandFlags {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	return commandFlags{
		fs:      fs,
		jsonOut: fs.Bool("json", false, "print machine-readable JSON"),
	}
}

func (c commandFlags) parse(args []string) (bool, error) {
	if err := c.fs.Parse(args); err != nil {
		return false, UsageError("%v", err)
	}
	return *c.jsonOut, nil
}

type paneCommandFlags struct {
	commandFlags
	pane *string
}

func (a *App) newPaneCommandFlags(name string) paneCommandFlags {
	command := a.newCommandFlags(name)
	return paneCommandFlags{
		commandFlags: command,
		pane:         command.fs.String("pane", "", "pane id or pane key (required)"),
	}
}

func (p paneCommandFlags) parse(args []string) (string, bool, error) {
	jsonOut, err := p.commandFlags.parse(args)
	if err != nil {
		return "", false, err
	}
	pane := strings.TrimSpace(*p.pane)
	if pane == "" {
		return "", false, UsageError("%s requires --pane", p.fs.Name())
	}
	return pane, jsonOut, nil
}

func (a *App) writeOutput(jsonOut bool, payload any, writeText func() error) error {
	if jsonOut {
		return writeJSON(a.stdout, payload)
	}
	return writeText()
}
