package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/portgle/tmux-connect/internal/tagb"
	"github.com/portgle/tmux-connect/internal/tmux"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	socket, args, err := parseGlobalArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(tagb.ExitUsage)
	}

	app := tagb.NewApp(os.Stdout, os.Stderr, tagb.NewService(tmux.NewClient(tmux.RealRunner{}, socket)))
	if err := app.Run(ctx, args); err != nil {
		code := tagb.ExitCode(err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(code)
	}
}

func parseGlobalArgs(args []string) (string, []string, error) {
	socket := strings.TrimSpace(os.Getenv("TAGB_TMUX_SOCKET"))
	jsonOut := false
	remaining := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--socket" || arg == "-L":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("%s requires a value", arg)
			}
			socket = strings.TrimSpace(args[i+1])
			i++
		case strings.HasPrefix(arg, "--socket="):
			socket = strings.TrimSpace(strings.TrimPrefix(arg, "--socket="))
		case arg == "--json":
			jsonOut = true
		default:
			remaining = append(remaining, args[i:]...)
			if jsonOut {
				remaining = append([]string{remaining[0], "--json"}, remaining[1:]...)
			}
			return socket, remaining, nil
		}
	}

	if jsonOut {
		return "", nil, fmt.Errorf("--json requires a command")
	}
	return socket, remaining, nil
}
