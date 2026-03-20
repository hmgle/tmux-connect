package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

const DefaultDirName = "tmux-connect"

type File struct {
	Tmux   Tmux   `toml:"tmux"`
	Serve  Serve  `toml:"serve"`
	Daemon Daemon `toml:"daemon"`
}

type Tmux struct {
	Socket *string `toml:"socket"`
}

type Serve struct {
	Listen *string `toml:"listen"`
}

type Daemon struct {
	Platform          *string   `toml:"platform"`
	DB                *string   `toml:"db"`
	AllowChats        *[]string `toml:"allow_chats"`
	PollTimeout       *string   `toml:"poll_timeout"`
	SnapshotLines     *int      `toml:"snapshot_lines"`
	FollowLines       *int      `toml:"follow_lines"`
	FollowMinInterval *string   `toml:"follow_min_interval"`
	FollowDebug       *bool     `toml:"follow_debug"`
	Telegram          Telegram  `toml:"telegram"`
	Slack             Slack     `toml:"slack"`
}

type Telegram struct {
	Token            *string  `toml:"token"`
	APIBase          *string  `toml:"api_base"`
	SnapshotTheme    *string  `toml:"snapshot_theme"`
	SnapshotFontSize *float64 `toml:"snapshot_font_size"`
	SnapshotFontFile *string  `toml:"snapshot_font_file"`
}

type Slack struct {
	BotToken      *string `toml:"bot_token"`
	AppToken      *string `toml:"app_token"`
	CommandPrefix *string `toml:"command_prefix"`
}

type Loaded struct {
	Path   string
	Config File
	Exists bool
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(dir, DefaultDirName, "config.toml"), nil
}

func Load(path string) (Loaded, error) {
	explicit := strings.TrimSpace(path) != ""
	if !explicit {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return Loaded{}, err
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !explicit {
			return Loaded{Path: path}, nil
		}
		return Loaded{}, fmt.Errorf("read config file %s: %w", path, err)
	}

	cfg := File{}
	if err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&cfg); err != nil {
		return Loaded{}, fmt.Errorf("decode config file %s: %w", path, err)
	}

	return Loaded{
		Path:   path,
		Config: cfg,
		Exists: true,
	}, nil
}

func ExtractPath(args []string) (string, []string, error) {
	var path string
	remaining := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--config":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("--config requires a value")
			}
			path = strings.TrimSpace(args[i+1])
			if path == "" {
				return "", nil, fmt.Errorf("--config requires a value")
			}
			i++
		case strings.HasPrefix(arg, "--config="):
			path = strings.TrimSpace(strings.TrimPrefix(arg, "--config="))
			if path == "" {
				return "", nil, fmt.Errorf("--config requires a value")
			}
		case arg == "--socket" || arg == "-L":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("%s requires a value", arg)
			}
			remaining = append(remaining, arg, args[i+1])
			i++
		case strings.HasPrefix(arg, "--socket="), arg == "--json":
			remaining = append(remaining, arg)
		default:
			remaining = append(remaining, args[i:]...)
			return path, remaining, nil
		}
	}

	return path, remaining, nil
}
