package daemon

import (
	"flag"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/hmgle/tmux-connect/internal/config"
	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

func parseConfig(args []string, stderr io.Writer, requireRun bool) (Config, error) {
	return parseConfigWithFile(args, stderr, requireRun, config.Daemon{})
}

func parseConfigWithFile(args []string, stderr io.Writer, requireRun bool, fileCfg config.Daemon) (Config, error) {
	defaults, err := resolveConfigDefaults(fileCfg)
	if err != nil {
		return Config{}, err
	}

	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg := Config{}
	values := bindConfigFlags(fs, &cfg, defaults)

	if err := fs.Parse(args); err != nil {
		return Config{}, tmuxconn.UsageError("%v", err)
	}
	if err := applyConfigValues(fs, &cfg, values, fileCfg); err != nil {
		return Config{}, err
	}
	if err := validateConfig(&cfg, fs, fileCfg, requireRun); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func defaultWhatsAppSessionDBPath(dbPath string) string {
	dbPath = strings.TrimSpace(dbPath)
	if dbPath == "" {
		return filepath.Join(filepath.Dir(defaultDBPath()), "whatsapp-device.db")
	}
	return filepath.Join(filepath.Dir(dbPath), "whatsapp-device.db")
}

func flagWasSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

func normalizePlainTextConfig(cfg PlainTextConfig) PlainTextConfig {
	if cfg.Mode == "" {
		cfg.Mode = plainTextModeType
	}
	if cfg.Echo == "" {
		cfg.Echo = plainTextEchoSnapshot
	}
	if cfg.EchoLines <= 0 {
		cfg.EchoLines = 12
	}
	if cfg.EchoDelay <= 0 {
		cfg.EchoDelay = 250 * time.Millisecond
	}
	if cfg.EchoTimeout <= 0 {
		cfg.EchoTimeout = 2 * time.Second
	}
	return cfg
}
