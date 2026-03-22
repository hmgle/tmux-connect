package daemon

import (
	"html"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/hmgle/tmux-connect/internal/tmux"
	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

func formatCurrent(record tmuxconn.PaneRecord, following bool) string {
	lines := []string{
		"Current pane: " + displayPaneKey(record.Info.Target.PaneKey()),
		"Where: " + formatPaneWhere(record.Info),
		"Command: " + displayValue(record.Info.CurrentCmd),
		"Dir: " + formatPaneDir(record.Info.CurrentPath),
		"Follow: " + onOff(following),
	}
	return strings.Join(lines, "\n")
}

func formatPaneList(records []tmuxconn.PaneRecord, current string, following bool) string {
	if len(records) == 0 {
		return "No managed panes.\n\nCurrent: none · Follow: " + onOff(following)
	}

	type row struct {
		marker string
		pane   string
		cmd    string
		dir    string
		where  string
	}

	rows := make([]row, len(records))
	header := row{marker: " ", pane: "Pane", cmd: "Cmd", dir: "Dir", where: "Where"}
	wPane := utf8.RuneCountInString(header.pane)
	wCmd := utf8.RuneCountInString(header.cmd)
	wDir := utf8.RuneCountInString(header.dir)

	for i, rec := range records {
		r := row{
			marker: " ",
			pane:   displayPaneKey(rec.Info.Target.PaneKey()),
			cmd:    shortenDisplay(displayValue(rec.Info.CurrentCmd), 16),
			dir:    formatPaneDir(rec.Info.CurrentPath),
			where:  shortenDisplay(formatPaneWhere(rec.Info), 20),
		}
		if rec.Info.Target.PaneKey() == current {
			r.marker = ">"
		}
		rows[i] = r
		if n := utf8.RuneCountInString(r.pane); n > wPane {
			wPane = n
		}
		if n := utf8.RuneCountInString(r.cmd); n > wCmd {
			wCmd = n
		}
		if n := utf8.RuneCountInString(r.dir); n > wDir {
			wDir = n
		}
	}

	var b strings.Builder
	b.WriteString("<b>Panes:</b>\n<pre>")
	writeRow := func(r row) {
		b.WriteString(r.marker)
		b.WriteByte(' ')
		writePadded(&b, r.pane, wPane)
		b.WriteString("  ")
		writePadded(&b, r.cmd, wCmd)
		b.WriteString("  ")
		writePadded(&b, r.dir, wDir)
		b.WriteString("  ")
		b.WriteString(html.EscapeString(r.where))
	}
	writeRow(header)
	for _, r := range rows {
		b.WriteByte('\n')
		writeRow(r)
	}
	b.WriteString("</pre>\nCurrent: ")
	b.WriteString(html.EscapeString(displayCurrent(current)))
	b.WriteString(" · Follow: ")
	b.WriteString(onOff(following))
	return b.String()
}

func writePadded(b *strings.Builder, s string, width int) {
	b.WriteString(html.EscapeString(s))
	for i := utf8.RuneCountInString(s); i < width; i++ {
		b.WriteByte(' ')
	}
}

func displayCurrent(current string) string {
	current = strings.TrimSpace(current)
	if current == "" {
		return "none"
	}
	return displayPaneKey(current)
}

func displayPaneKey(paneKey string) string {
	paneKey = strings.TrimSpace(paneKey)
	if paneKey == "" {
		return "-"
	}
	if strings.HasPrefix(paneKey, "default:") {
		return strings.TrimPrefix(paneKey, "default:")
	}
	return paneKey
}

func formatPaneDir(currentPath string) string {
	currentPath = strings.TrimSpace(currentPath)
	if currentPath == "" {
		return "-"
	}
	dirName := filepath.Base(filepath.Clean(currentPath))
	if strings.TrimSpace(dirName) == "" {
		dirName = currentPath
	}
	return shortenDisplay(dirName, 14)
}

func formatPaneWhere(info tmux.PaneInfo) string {
	return strings.TrimSpace(displayValue(info.SessionName) + "/" + displayValue(info.WindowName))
}

func displayValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func shortenDisplay(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	runes := []rune(value)
	if len(runes) <= maxRunes || maxRunes <= 0 {
		return value
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	remaining := maxRunes - 3
	prefix := remaining / 2
	suffix := remaining - prefix
	if prefix == 0 {
		return "..." + string(runes[len(runes)-suffix:])
	}
	if suffix == 0 {
		return string(runes[:prefix]) + "..."
	}
	return string(runes[:prefix]) + "..." + string(runes[len(runes)-suffix:])
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}
