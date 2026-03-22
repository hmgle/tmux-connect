package termrender

import (
	"image/color"
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
