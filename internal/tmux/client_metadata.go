package tmux

import (
	"bufio"
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

func (c *Client) GetUserOptions(ctx context.Context, target Target) (map[string]string, error) {
	output, err := c.run(ctx, nil, "show-options", "-p", "-t", target.PaneID)
	if err != nil {
		if errors.Is(classifyOptionError(err), ErrTmuxOptionUnavailable) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	return parseUserOptions(output), nil
}

func (c *Client) SetUserOption(ctx context.Context, target Target, key string, value string) error {
	_, err := c.run(ctx, nil, "set-option", "-p", "-t", target.PaneID, key, value)
	return err
}

func (c *Client) GetUserOption(ctx context.Context, target Target, key string) (string, error) {
	output, err := c.run(ctx, nil, "show-options", "-p", "-v", "-t", target.PaneID, key)
	if err != nil {
		if errors.Is(classifyOptionError(err), ErrTmuxOptionUnavailable) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func (c *Client) DeleteUserOption(ctx context.Context, target Target, key string) error {
	_, err := c.run(ctx, nil, "set-option", "-p", "-u", "-t", target.PaneID, key)
	if errors.Is(classifyOptionError(err), ErrTmuxOptionUnavailable) {
		return nil
	}
	return err
}

func (c *Client) GetMetadata(ctx context.Context, target Target) (BridgeMetadata, error) {
	opts, err := c.GetUserOptions(ctx, target)
	if err != nil {
		return BridgeMetadata{}, err
	}
	return MetadataFromOptions(opts), nil
}

func (c *Client) SetMetadata(ctx context.Context, target Target, meta BridgeMetadata) error {
	return c.runOptionCommands(ctx, target, false, metadataOptionPairs(meta))
}

func (c *Client) ClearMetadata(ctx context.Context, target Target) error {
	return c.runOptionCommands(ctx, target, true, metadataOptionPairs(BridgeMetadata{}))
}

func (c *Client) TouchMetadata(ctx context.Context, target Target) error {
	managed, err := c.GetUserOption(ctx, target, OptionManaged)
	if err != nil {
		return err
	}
	if managed != "1" {
		return nil
	}
	return c.TouchMetadataManaged(ctx, target)
}

func (c *Client) TouchMetadataManaged(ctx context.Context, target Target) error {
	return c.SetUserOption(ctx, target, OptionLastActivity, strconv.FormatInt(time.Now().Unix(), 10))
}

func (c *Client) runOptionCommands(ctx context.Context, target Target, unset bool, pairs []optionValue) error {
	commands := make([][]string, 0, len(pairs))
	for _, pair := range pairs {
		if unset {
			commands = append(commands, []string{"set-option", "-p", "-u", "-t", target.PaneID, pair.key})
			continue
		}
		commands = append(commands, []string{"set-option", "-p", "-t", target.PaneID, pair.key, pair.value})
	}
	if len(commands) == 0 {
		return nil
	}
	_, err := c.run(ctx, nil, joinTmuxCommands(commands)...)
	if unset && errors.Is(classifyOptionError(err), ErrTmuxOptionUnavailable) {
		return nil
	}
	return err
}

func joinTmuxCommands(commands [][]string) []string {
	args := make([]string, 0, len(commands)*8)
	for i, command := range commands {
		if i > 0 {
			args = append(args, ";")
		}
		args = append(args, command...)
	}
	return args
}

type optionValue struct {
	key   string
	value string
}

func metadataOptionPairs(meta BridgeMetadata) []optionValue {
	opts := meta.ToOptions()
	return []optionValue{
		{key: OptionManaged, value: opts[OptionManaged]},
		{key: OptionMode, value: opts[OptionMode]},
		{key: OptionAgent, value: opts[OptionAgent]},
		{key: OptionLabel, value: opts[OptionLabel]},
		{key: OptionCreatedBy, value: opts[OptionCreatedBy]},
		{key: OptionLastActivity, value: opts[OptionLastActivity]},
	}
}

func parseUserOptions(output string) map[string]string {
	opts := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "@tmuxconn_") {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 1 {
			opts[parts[0]] = ""
			continue
		}
		opts[parts[0]] = strings.TrimSpace(parts[1])
	}
	return opts
}
