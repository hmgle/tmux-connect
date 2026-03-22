package termrender

import (
	"image/color"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/width"
)

type styledCell struct {
	r            rune
	style        style
	span         int
	continuation bool
}

type style struct {
	fg      color.RGBA
	bg      color.RGBA
	bold    bool
	reverse bool
}

func (s style) resolve() (color.RGBA, color.RGBA) {
	fg := s.fg
	bg := s.bg
	if s.reverse {
		fg, bg = bg, fg
	}
	if s.bold {
		fg = brighten(fg, 18)
	}
	return fg, bg
}

func brighten(c color.RGBA, amount uint8) color.RGBA {
	return rgba(
		min(int(c.R)+int(amount), 255),
		min(int(c.G)+int(amount), 255),
		min(int(c.B)+int(amount), 255),
	)
}

func parseANSI(text string, theme Theme) ([][]styledCell, error) {
	lines := make([][]styledCell, 1)
	current := defaultStyle(theme)
	row := 0
	col := 0

	for i := 0; i < len(text); {
		switch text[i] {
		case '\x1b':
			if i+1 < len(text) && text[i+1] == '[' {
				end := i + 2
				for end < len(text) && !isCSIEnd(text[end]) {
					end++
				}
				if end >= len(text) {
					i = end
					continue
				}
				if text[end] == 'm' {
					current = applySGR(current, parseSGRParams(text[i+2:end]), theme)
				}
				i = end + 1
				continue
			}
			i++
		case '\n':
			lines = append(lines, nil)
			row++
			col = 0
			i++
		case '\r':
			col = 0
			i++
		case '\t':
			nextTab := ((col / 8) + 1) * 8
			for col < nextTab {
				writeCell(lines, row, col, styledCell{r: ' ', style: current}, theme)
				col++
			}
			i++
		case '\b':
			if col > 0 {
				col--
			}
			i++
		default:
			r, size := rune(text[i]), 1
			if r >= utf8.RuneSelf {
				r, size = utf8.DecodeRuneInString(text[i:])
			}
			if r == utf8.RuneError && size == 1 {
				r = '?'
				size = 1
			}
			if !unicode.IsPrint(r) {
				i += size
				continue
			}
			span := runeCellSpan(r)
			writeCell(lines, row, col, styledCell{r: r, style: current, span: span}, theme)
			for offset := 1; offset < span; offset++ {
				writeCell(lines, row, col+offset, styledCell{style: current, continuation: true}, theme)
			}
			col += span
			i += size
		}
	}

	return lines, nil
}

func writeCell(lines [][]styledCell, row int, col int, cell styledCell, theme Theme) {
	line := lines[row]
	for len(line) <= col {
		line = append(line, blankCell(theme))
	}
	line[col] = cell
	lines[row] = line
}

func blankCell(theme Theme) styledCell {
	return styledCell{r: ' ', style: defaultStyle(theme), span: 1}
}

func runeCellSpan(r rune) int {
	switch width.LookupRune(r).Kind() {
	case width.EastAsianFullwidth, width.EastAsianWide:
		return 2
	default:
		return 1
	}
}

func isCSIEnd(b byte) bool {
	return b >= 0x40 && b <= 0x7e
}

func parseSGRParams(raw string) []int {
	if strings.TrimSpace(raw) == "" {
		return []int{0}
	}
	parts := strings.Split(raw, ";")
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			values = append(values, 0)
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			values = append(values, 0)
			continue
		}
		values = append(values, value)
	}
	return values
}

func applySGR(current style, params []int, theme Theme) style {
	if len(params) == 0 {
		return defaultStyle(theme)
	}
	for i := 0; i < len(params); i++ {
		switch code := params[i]; {
		case code == 0:
			current = defaultStyle(theme)
		case code == 1:
			current.bold = true
		case code == 22:
			current.bold = false
		case code == 7:
			current.reverse = true
		case code == 27:
			current.reverse = false
		case code == 39:
			current.fg = theme.Foreground
		case code == 49:
			current.bg = theme.Background
		case code >= 30 && code <= 37:
			current.fg = ansiColor(theme, code-30)
		case code >= 90 && code <= 97:
			current.fg = ansiColor(theme, code-90+8)
		case code >= 40 && code <= 47:
			current.bg = ansiColor(theme, code-40)
		case code >= 100 && code <= 107:
			current.bg = ansiColor(theme, code-100+8)
		case code == 38 || code == 48:
			isForeground := code == 38
			parsed, consumed := parseExtendedColor(params[i+1:], theme)
			if consumed == 0 {
				continue
			}
			if isForeground {
				current.fg = parsed
			} else {
				current.bg = parsed
			}
			i += consumed
		}
	}
	return current
}

func parseExtendedColor(params []int, theme Theme) (color.RGBA, int) {
	if len(params) < 2 {
		return theme.Foreground, 0
	}
	switch params[0] {
	case 5:
		return ansi256Color(theme, params[1]), 2
	case 2:
		if len(params) < 4 {
			return theme.Foreground, 0
		}
		return rgba(params[1], params[2], params[3]), 4
	default:
		return theme.Foreground, 0
	}
}

func defaultStyle(theme Theme) style {
	return style{
		fg: theme.Foreground,
		bg: theme.Background,
	}
}

func ansiColor(theme Theme, index int) color.RGBA {
	if index < 0 || index >= len(theme.Palette) {
		return theme.Foreground
	}
	return theme.Palette[index]
}

func ansi256Color(theme Theme, index int) color.RGBA {
	if index < 0 {
		index = 0
	}
	if index < 16 {
		return ansiColor(theme, index)
	}
	if index >= 232 {
		level := uint8((index-232)*10 + 8)
		return rgba(int(level), int(level), int(level))
	}
	index -= 16
	r := index / 36
	g := (index / 6) % 6
	b := index % 6
	return rgba(ansiCubeValue(r), ansiCubeValue(g), ansiCubeValue(b))
}

func ansiCubeValue(v int) int {
	if v == 0 {
		return 0
	}
	return 55 + v*40
}

func rgba(r int, g int, b int) color.RGBA {
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 0xff}
}
