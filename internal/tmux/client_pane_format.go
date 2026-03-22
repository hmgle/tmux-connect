package tmux

import (
	"fmt"
	"strconv"
	"strings"
)

func paneListFormat() string {
	return strings.Join([]string{
		"#{pane_id}",
		"#{session_name}",
		"#{window_id}",
		"#{window_name}",
		"#{pane_title}",
		"#{pane_current_command}",
		"#{pane_current_path}",
		"#{pane_dead}",
		"#{pane_width}",
		"#{pane_height}",
	}, paneFieldSep)
}

func paneStateFormat() string {
	return strings.Join([]string{
		"#{pane_id}",
		"#{session_name}",
		"#{window_id}",
		"#{window_name}",
		"#{pane_title}",
		"#{pane_current_command}",
		"#{pane_current_path}",
		"#{pane_dead}",
		"#{pane_width}",
		"#{pane_height}",
		"#{@tmuxconn_managed}",
		"#{@tmuxconn_mode}",
		"#{@tmuxconn_agent}",
		"#{@tmuxconn_label}",
		"#{@tmuxconn_created_by}",
		"#{@tmuxconn_last_activity_unix}",
	}, paneFieldSep)
}

func parsePaneStateLine(socket string, line string) (PaneInfo, BridgeMetadata, error) {
	fields := splitPaneFields(line, 16)
	if len(fields) != 16 {
		return PaneInfo{}, BridgeMetadata{}, fmt.Errorf("unexpected list-panes row: %q", line)
	}
	info, err := buildPaneInfo(socket, fields[:10])
	if err != nil {
		return PaneInfo{}, BridgeMetadata{}, err
	}
	meta := MetadataFromOptions(map[string]string{
		OptionManaged:      fields[10],
		OptionMode:         fields[11],
		OptionAgent:        fields[12],
		OptionLabel:        fields[13],
		OptionCreatedBy:    fields[14],
		OptionLastActivity: fields[15],
	})
	return info, meta, nil
}

func parsePaneInfoLine(socket string, line string) (PaneInfo, error) {
	fields := splitPaneFields(line, 10)
	if len(fields) != 10 {
		return PaneInfo{}, fmt.Errorf("unexpected list-panes row: %q", line)
	}
	return buildPaneInfo(socket, fields)
}

func splitPaneFields(line string, expected int) []string {
	fields := strings.Split(line, paneFieldSep)
	if len(fields) == expected {
		return fields
	}
	if strings.Contains(line, paneFieldSepEscaped) {
		fields = strings.Split(line, paneFieldSepEscaped)
		if len(fields) == expected {
			return fields
		}
	}
	return fields
}

func buildPaneInfo(socket string, fields []string) (PaneInfo, error) {
	width, err := strconv.Atoi(fields[8])
	if err != nil {
		return PaneInfo{}, fmt.Errorf("parse width for %s: %w", fields[0], err)
	}
	height, err := strconv.Atoi(fields[9])
	if err != nil {
		return PaneInfo{}, fmt.Errorf("parse height for %s: %w", fields[0], err)
	}
	return PaneInfo{
		Target:      Target{Socket: socket, PaneID: fields[0]},
		SessionName: fields[1],
		WindowID:    fields[2],
		WindowName:  fields[3],
		PaneTitle:   fields[4],
		CurrentCmd:  fields[5],
		CurrentPath: fields[6],
		Dead:        fields[7] == "1",
		Width:       width,
		Height:      height,
	}, nil
}

func paneOutputLines(output string) []string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	filtered := lines[:0]
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		filtered = append(filtered, line)
	}
	return filtered
}
