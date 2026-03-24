package weixin

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

func isMediaItemType(t int) bool {
	switch t {
	case messageItemImage, messageItemVoice, messageItemFile, messageItemVideo:
		return true
	default:
		return false
	}
}

func bodyFromItemList(items []messageItem) string {
	if len(items) == 0 {
		return ""
	}
	for _, item := range items {
		switch item.Type {
		case messageItemText:
			if item.TextItem == nil {
				continue
			}
			text := strings.TrimSpace(item.TextItem.Text)
			ref := item.RefMsg
			if ref == nil {
				return text
			}
			if ref.MessageItem != nil && isMediaItemType(ref.MessageItem.Type) {
				return text
			}
			var parts []string
			if ref.Title != "" {
				parts = append(parts, ref.Title)
			}
			if ref.MessageItem != nil {
				refBody := bodyFromItemList([]messageItem{*ref.MessageItem})
				if refBody != "" {
					parts = append(parts, refBody)
				}
			}
			if len(parts) == 0 {
				return text
			}
			return fmt.Sprintf("[引用: %s]\n%s", strings.Join(parts, " | "), text)
		case messageItemVoice:
			if item.VoiceItem != nil && strings.TrimSpace(item.VoiceItem.Text) != "" {
				return strings.TrimSpace(item.VoiceItem.Text)
			}
		}
	}
	return ""
}

func splitUTF8(s string, maxRunes int) []string {
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return []string{s}
	}
	var out []string
	runes := []rune(s)
	for len(runes) > 0 {
		n := maxRunes
		if len(runes) < n {
			n = len(runes)
		}
		out = append(out, string(runes[:n]))
		runes = runes[n:]
	}
	return out
}
