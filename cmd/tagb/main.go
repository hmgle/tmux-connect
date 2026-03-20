package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tagbconfig "github.com/hmgle/tmux-connect/internal/config"
	"github.com/hmgle/tmux-connect/internal/daemon"
	"github.com/hmgle/tmux-connect/internal/httpapi"
	"github.com/hmgle/tmux-connect/internal/tagb"
	"github.com/hmgle/tmux-connect/internal/tmux"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	configPath, args, err := tagbconfig.ExtractPath(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(tagb.ExitUsage)
	}

	loadedConfig, err := tagbconfig.Load(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(tagb.ExitUsage)
	}

	socket, args, err := parseGlobalArgs(args, loadedConfig.Config)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(tagb.ExitUsage)
	}

	service := tagb.NewService(tmux.NewClient(tmux.RealRunner{}, socket))
	app := tagb.NewApp(os.Stdout, os.Stderr, service)
	if len(args) > 0 && args[0] == "serve" {
		if err := runServe(ctx, os.Stdout, os.Stderr, service, loadedConfig.Config.Serve, args[1:]); err != nil {
			code := tagb.ExitCode(err)
			fmt.Fprintln(os.Stderr, err)
			os.Exit(code)
		}
		return
	}
	if len(args) > 0 && args[0] == "daemon" {
		if err := daemon.RunCLIWithConfig(ctx, os.Stdout, os.Stderr, service, loadedConfig.Config.Daemon, args[1:]); err != nil {
			code := tagb.ExitCode(err)
			fmt.Fprintln(os.Stderr, err)
			os.Exit(code)
		}
		return
	}
	if err := app.Run(ctx, args); err != nil {
		code := tagb.ExitCode(err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(code)
	}
}

func runServe(ctx context.Context, stdout io.Writer, stderr io.Writer, service *tagb.Service, fileCfg tagbconfig.Serve, args []string) error {
	listen, err := parseServeArgs(stderr, fileCfg, args)
	if err != nil {
		return err
	}
	server := httpapi.NewServer(listen, service)
	if _, err := fmt.Fprintf(stdout, "serving HTTP API on %s\n", server.Addr()); err != nil {
		return err
	}
	if err := server.ListenAndServe(ctx); err != nil {
		return tagb.TmuxError("serve http api: %v", err)
	}
	return nil
}

func parseServeArgs(stderr io.Writer, fileCfg tagbconfig.Serve, args []string) (string, error) {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listenDefault := "127.0.0.1:8080"
	if fileCfg.Listen != nil {
		listenDefault = strings.TrimSpace(*fileCfg.Listen)
	}
	listen := fs.String("listen", listenDefault, "HTTP listen address")
	if err := fs.Parse(args); err != nil {
		return "", tagb.UsageError("%v", err)
	}
	if strings.TrimSpace(*listen) == "" {
		return "", tagb.UsageError("serve requires --listen or [serve].listen in config")
	}
	if _, err := net.ResolveTCPAddr("tcp", *listen); err != nil {
		return "", tagb.UsageError("invalid listen address %q: %v", *listen, err)
	}
	return *listen, nil
}

func parseGlobalArgs(args []string, fileCfg tagbconfig.File) (string, []string, error) {
	socket := ""
	if fileCfg.Tmux.Socket != nil {
		socket = strings.TrimSpace(*fileCfg.Tmux.Socket)
	}
	if value := strings.TrimSpace(os.Getenv("TAGB_TMUX_SOCKET")); value != "" {
		socket = value
	}
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
