//go:build !no_weixin

package daemon

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hmgle/tmux-connect/internal/config"
	"github.com/hmgle/tmux-connect/internal/tmuxconn"
	"github.com/hmgle/tmux-connect/internal/weixin"
	"github.com/mdp/qrterminal/v3"
)

const (
	weixinSetupModeAuto = "auto"
	weixinSetupModeNew  = "new"
	weixinSetupModeBind = "bind"
	maxWeixinQRRefresh  = 3
)

type weixinCLIOptions struct {
	token             string
	apiURL            string
	cdnURL            string
	timeout           time.Duration
	routeTag          string
	botType           string
	setAllowChatEmpty bool
	skipVerify        bool
}

type weixinCLIResult struct {
	token         string
	baseURL       string
	cdnBaseURL    string
	routeTag      string
	scannedUserID string
}

type weixinCLIDeps struct {
	httpClient func() *http.Client
	sleep      func(time.Duration)
}

func defaultWeixinCLIDeps() weixinCLIDeps {
	return weixinCLIDeps{
		httpClient: func() *http.Client { return &http.Client{} },
		sleep:      time.Sleep,
	}
}

func runWeixinCLIWithLoadedConfig(ctx context.Context, stdout io.Writer, stderr io.Writer, loaded config.Loaded, args []string) error {
	return runWeixinCLIWithDeps(ctx, stdout, stderr, loaded, args, defaultWeixinCLIDeps())
}

func runWeixinCLIWithDeps(ctx context.Context, stdout io.Writer, stderr io.Writer, loaded config.Loaded, args []string, deps weixinCLIDeps) error {
	if len(args) == 0 {
		printWeixinUsage(stdout)
		return nil
	}

	switch args[0] {
	case "setup":
		return runWeixinSetupCommand(ctx, stdout, stderr, loaded, args[1:], weixinSetupModeAuto, deps)
	case "new", "create":
		return runWeixinSetupCommand(ctx, stdout, stderr, loaded, args[1:], weixinSetupModeNew, deps)
	case "bind", "link":
		return runWeixinSetupCommand(ctx, stdout, stderr, loaded, args[1:], weixinSetupModeBind, deps)
	case "help", "-h", "--help":
		printWeixinUsage(stdout)
		return nil
	default:
		printWeixinUsage(stderr)
		return tmuxconn.UsageError("unknown daemon weixin command: %s", args[0])
	}
}

func runWeixinSetupCommand(ctx context.Context, stdout io.Writer, stderr io.Writer, loaded config.Loaded, args []string, requestedMode string, deps weixinCLIDeps) error {
	fs := flag.NewFlagSet("daemon weixin "+requestedMode, flag.ContinueOnError)
	fs.SetOutput(stderr)

	var opts weixinCLIOptions
	fs.StringVar(&opts.token, "token", "", "existing ilink bot bearer token")
	fs.StringVar(&opts.apiURL, "api-url", defaultWeixinBaseURL, "ilink api base url")
	fs.StringVar(&opts.cdnURL, "cdn-url", defaultWeixinCDNBaseURL, "ilink cdn base url to save in config")
	fs.DurationVar(&opts.timeout, "timeout", 8*time.Minute, "qr login timeout")
	fs.StringVar(&opts.routeTag, "route-tag", "", "optional SKRouteTag header")
	fs.StringVar(&opts.botType, "bot-type", weixin.DefaultBotType, "bot_type query param for get_bot_qrcode")
	fs.BoolVar(&opts.setAllowChatEmpty, "set-allow-chat-empty", true, "set allow_chats to the scanned weixin user when currently empty")
	fs.BoolVar(&opts.skipVerify, "skip-verify", false, "bind mode: skip getupdates token verification")
	if err := fs.Parse(args); err != nil {
		if tmuxconn.IsHelpError(err) {
			return err
		}
		return tmuxconn.UsageError("%v", err)
	}

	mode, err := resolveWeixinSetupMode(requestedMode, opts.token)
	if err != nil {
		return tmuxconn.UsageError("%v", err)
	}

	var result weixinCLIResult
	switch mode {
	case weixinSetupModeBind:
		result, err = runWeixinBindFlow(ctx, opts, deps)
	case weixinSetupModeNew:
		result, err = runWeixinQRLoginFlow(ctx, stdout, stderr, opts, deps)
	default:
		err = fmt.Errorf("unsupported weixin setup mode %q", mode)
	}
	if err != nil {
		return err
	}

	updated := loaded.Config
	updated.Daemon.Platform = stringValuePtr("weixin")
	updated.Daemon.Weixin.Token = stringValuePtr(result.token)
	updated.Daemon.Weixin.BaseURL = stringValuePtr(result.baseURL)
	updated.Daemon.Weixin.CDNBaseURL = stringValuePtr(result.cdnBaseURL)
	if result.routeTag != "" {
		updated.Daemon.Weixin.RouteTag = stringValuePtr(result.routeTag)
	} else {
		updated.Daemon.Weixin.RouteTag = nil
	}
	if opts.setAllowChatEmpty && result.scannedUserID != "" && allowChatsEmpty(updated.Daemon.AllowChats) {
		updated.Daemon.AllowChats = &[]string{"weixin:" + result.scannedUserID}
	}

	savedPath, err := config.Save(loaded.Path, updated)
	if err != nil {
		return err
	}

	fmt.Fprintln(stdout, "weixin setup: configuration updated")
	fmt.Fprintf(stdout, "config: %s\n", savedPath)
	fmt.Fprintf(stdout, "platform: weixin\n")
	fmt.Fprintf(stdout, "api base: %s\n", result.baseURL)
	if updated.Daemon.AllowChats != nil && len(*updated.Daemon.AllowChats) > 0 {
		fmt.Fprintf(stdout, "allow_chats: %s\n", strings.Join(*updated.Daemon.AllowChats, ","))
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Next: run `tmux-connect daemon run --platform weixin ...` and send one message from Weixin to establish context_token.")
	return nil
}

func runWeixinBindFlow(ctx context.Context, opts weixinCLIOptions, deps weixinCLIDeps) (weixinCLIResult, error) {
	token := strings.TrimSpace(opts.token)
	if token == "" {
		return weixinCLIResult{}, tmuxconn.UsageError("bind mode requires --token")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(opts.apiURL), "/")
	if baseURL == "" {
		baseURL = defaultWeixinBaseURL
	}
	if !opts.skipVerify {
		verifyCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if err := weixin.VerifyToken(verifyCtx, deps.httpClient(), baseURL, token, opts.routeTag); err != nil {
			return weixinCLIResult{}, tmuxconn.UsageError("weixin token verification failed: %v", err)
		}
	}
	return weixinCLIResult{
		token:      token,
		baseURL:    baseURL,
		cdnBaseURL: normalizedWeixinCDNURL(opts.cdnURL),
		routeTag:   strings.TrimSpace(opts.routeTag),
	}, nil
}

func runWeixinQRLoginFlow(ctx context.Context, stdout io.Writer, stderr io.Writer, opts weixinCLIOptions, deps weixinCLIDeps) (weixinCLIResult, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(opts.apiURL), "/")
	if baseURL == "" {
		baseURL = defaultWeixinBaseURL
	}
	qr, err := weixin.FetchBotQRCode(ctx, deps.httpClient(), baseURL, opts.botType, opts.routeTag)
	if err != nil {
		return weixinCLIResult{}, tmuxconn.UsageError("weixin qr fetch failed: %v", err)
	}
	qrURL := strings.TrimSpace(qr.QRCodeImgContent)
	if qrURL == "" {
		return weixinCLIResult{}, tmuxconn.UsageError("weixin qr fetch returned empty qrcode_img_content")
	}
	fmt.Fprintln(stdout, "Scan this Weixin iLink QR code:")
	fmt.Fprintf(stdout, "URL: %s\n\n", qrURL)
	qrterminal.GenerateHalfBlock(qrURL, qrterminal.L, stdout)
	fmt.Fprintln(stdout)

	deadline := time.Now().Add(opts.timeout)
	qrKey := strings.TrimSpace(qr.QRCode)
	refreshCount := 1
	scannedPrinted := false

	for time.Now().Before(deadline) {
		status, err := weixin.PollQRCodeStatus(ctx, deps.httpClient(), baseURL, qrKey, opts.routeTag)
		if err != nil {
			return weixinCLIResult{}, tmuxconn.UsageError("weixin qr status failed: %v", err)
		}
		switch strings.TrimSpace(status.Status) {
		case "", "wait":
			deps.sleep(time.Second)
			continue
		case "scaned":
			if !scannedPrinted {
				fmt.Fprintln(stdout, "\nQR scanned. Confirm login in Weixin...")
				scannedPrinted = true
			}
			deps.sleep(time.Second)
			continue
		case "expired":
			refreshCount++
			if refreshCount > maxWeixinQRRefresh {
				return weixinCLIResult{}, tmuxconn.UsageError("weixin qr expired too many times; retry setup")
			}
			fmt.Fprintf(stdout, "\nQR expired. Refreshing (%d/%d)...\n", refreshCount, maxWeixinQRRefresh)
			refreshed, err := weixin.FetchBotQRCode(ctx, deps.httpClient(), baseURL, opts.botType, opts.routeTag)
			if err != nil {
				return weixinCLIResult{}, tmuxconn.UsageError("weixin qr refresh failed: %v", err)
			}
			qrKey = strings.TrimSpace(refreshed.QRCode)
			qrURL = strings.TrimSpace(refreshed.QRCodeImgContent)
			scannedPrinted = false
			if qrURL != "" {
				fmt.Fprintf(stdout, "URL: %s\n\n", qrURL)
				qrterminal.GenerateHalfBlock(qrURL, qrterminal.L, stdout)
				fmt.Fprintln(stdout)
			}
			deps.sleep(time.Second)
		case "confirmed":
			token := strings.TrimSpace(status.BotToken)
			if token == "" {
				return weixinCLIResult{}, tmuxconn.UsageError("weixin login confirmed but bot_token is missing")
			}
			resolvedBaseURL := strings.TrimRight(strings.TrimSpace(status.BaseURL), "/")
			if resolvedBaseURL == "" {
				resolvedBaseURL = baseURL
			}
			fmt.Fprintln(stdout, "\nWeixin iLink login confirmed.")
			return weixinCLIResult{
				token:         token,
				baseURL:       resolvedBaseURL,
				cdnBaseURL:    normalizedWeixinCDNURL(opts.cdnURL),
				routeTag:      strings.TrimSpace(opts.routeTag),
				scannedUserID: strings.TrimSpace(status.IlinkUserID),
			}, nil
		default:
			fmt.Fprintf(stderr, "weixin login event: %s\n", status.Status)
			deps.sleep(time.Second)
		}
	}
	return weixinCLIResult{}, tmuxconn.UsageError("waiting for weixin qr scan timed out")
}

func resolveWeixinSetupMode(requested string, token string) (string, error) {
	switch requested {
	case weixinSetupModeAuto:
		if strings.TrimSpace(token) != "" {
			return weixinSetupModeBind, nil
		}
		return weixinSetupModeNew, nil
	case weixinSetupModeBind:
		if strings.TrimSpace(token) == "" {
			return "", fmt.Errorf("bind mode requires --token")
		}
		return weixinSetupModeBind, nil
	case weixinSetupModeNew:
		if strings.TrimSpace(token) != "" {
			return "", fmt.Errorf("new mode does not accept --token; use `tmux-connect daemon weixin bind --token ...`")
		}
		return weixinSetupModeNew, nil
	default:
		return "", fmt.Errorf("unknown weixin setup mode %q", requested)
	}
}

func allowChatsEmpty(chats *[]string) bool {
	return chats == nil || len(*chats) == 0
}

func normalizedWeixinCDNURL(value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" {
		value = defaultWeixinCDNBaseURL
	}
	return value
}

func printWeixinUsage(w io.Writer) {
	fmt.Fprintln(w, `tmux-connect daemon weixin manages Weixin iLink setup for the daemon config.

Usage:
  tmux-connect daemon weixin <command> [flags]

Commands:
  setup   QR login when no --token; with --token => bind an existing iLink token
  new     Force QR login (rejects --token)
  bind    Force token bind (requires --token)

Common flags:
  --token TOKEN
  --api-url URL
  --cdn-url URL
  --timeout 8m
  --route-tag TAG
  --bot-type 3
  --set-allow-chat-empty
  --skip-verify

Examples:
  tmux-connect daemon weixin setup
  tmux-connect daemon weixin bind --token eyJ...
  tmux-connect daemon weixin setup --token eyJ...`)
}

func stringValuePtr(value string) *string {
	return &value
}
