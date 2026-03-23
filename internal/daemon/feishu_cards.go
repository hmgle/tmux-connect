package daemon

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

const feishuPaneCardLimit = 20

type feishuCard struct {
	Config   *feishuCardConfig   `json:"config,omitempty"`
	Header   *feishuCardHeader   `json:"header,omitempty"`
	Elements []feishuCardElement `json:"elements,omitempty"`
}

type feishuCardConfig struct {
	WideScreenMode bool `json:"wide_screen_mode,omitempty"`
}

type feishuCardHeader struct {
	Template string               `json:"template,omitempty"`
	Title    *feishuCardPlainText `json:"title,omitempty"`
}

type feishuCardPlainText struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

type feishuCardElement map[string]any

func buildFeishuHelpCard(helpText string) string {
	helpText = strings.TrimSpace(helpText)
	sections := splitFeishuCardSections(helpText)
	elements := make([]feishuCardElement, 0, len(sections)*2)
	for idx, section := range sections {
		if idx > 0 {
			elements = append(elements, feishuCardElement{"tag": "hr"})
		}
		elements = append(elements, feishuCardElement{
			"tag":     "markdown",
			"content": section,
		})
	}
	return mustMarshalFeishuCard(feishuCard{
		Config: &feishuCardConfig{WideScreenMode: true},
		Header: &feishuCardHeader{
			Template: "blue",
			Title: &feishuCardPlainText{
				Tag:     "plain_text",
				Content: "tmux-connect Help",
			},
		},
		Elements: elements,
	})
}

func buildFeishuPaneChoiceCard(command string, records []tmuxconn.PaneRecord) string {
	command = strings.TrimSpace(command)
	title := "Select A Pane"
	intro := "Reply with a pane number or pane id, for example `1` or `%5`."
	if command == "unmanage" {
		title = "Choose A Pane To Unmanage"
		intro = "Reply with a pane number or pane id to stop managing it."
	}

	lines := []string{intro, "", "Available panes:"}
	limit := len(records)
	if limit > feishuPaneCardLimit {
		limit = feishuPaneCardLimit
	}
	for idx := 0; idx < limit; idx++ {
		record := records[idx]
		lines = append(lines, fmt.Sprintf("%d. `%s`  %s / %s", idx+1, record.Info.Target.PaneKey(), record.Info.SessionName, record.Info.WindowName))
	}
	if len(records) > limit {
		lines = append(lines, "", fmt.Sprintf("Only the first %d panes are shown. You can still reply with a full pane id directly.", limit))
	}
	lines = append(lines, "", "You can also reply with `default:%5` directly.")

	return mustMarshalFeishuCard(feishuCard{
		Config: &feishuCardConfig{WideScreenMode: true},
		Header: &feishuCardHeader{
			Template: "wathet",
			Title: &feishuCardPlainText{
				Tag:     "plain_text",
				Content: title,
			},
		},
		Elements: []feishuCardElement{
			{
				"tag":     "markdown",
				"content": strings.Join(lines, "\n"),
			},
		},
	})
}

func splitFeishuCardSections(text string) []string {
	lines := strings.Split(text, "\n")
	sections := make([]string, 0, 4)
	current := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			if len(current) > 0 {
				sections = append(sections, strings.Join(current, "\n"))
				current = current[:0]
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		sections = append(sections, strings.Join(current, "\n"))
	}
	if len(sections) == 0 {
		return []string{"No help text available."}
	}
	return sections
}

func mustMarshalFeishuCard(card feishuCard) string {
	data, err := json.Marshal(card)
	if err != nil {
		panic(err)
	}
	return string(data)
}
