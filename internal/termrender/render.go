package termrender

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type Options struct {
	FontSize float64
	PaddingX int
	PaddingY int
}

type Theme struct {
	Foreground color.RGBA
	Background color.RGBA
}

var defaultTheme = Theme{
	Foreground: rgba(217, 224, 238),
	Background: rgba(17, 24, 39),
}

type styledCell struct {
	r     rune
	style style
}

type style struct {
	fg      color.RGBA
	bg      color.RGBA
	bold    bool
	reverse bool
}

var (
	fontOnce   sync.Once
	fontErr    error
	parsedMono *opentype.Font
)

func RenderPNG(text string, opts Options) ([]byte, error) {
	lines, err := parseANSI(text)
	if err != nil {
		return nil, err
	}
	opts = normalizeOptions(opts)
	face, err := loadFace(opts.FontSize)
	if err != nil {
		return nil, err
	}
	defer face.Close()

	img, err := render(lines, face, opts)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}

func normalizeOptions(opts Options) Options {
	if opts.FontSize <= 0 {
		opts.FontSize = 14
	}
	if opts.PaddingX <= 0 {
		opts.PaddingX = 14
	}
	if opts.PaddingY <= 0 {
		opts.PaddingY = 12
	}
	return opts
}

func loadFace(size float64) (font.Face, error) {
	fontOnce.Do(func() {
		parsedMono, fontErr = opentype.Parse(gomono.TTF)
	})
	if fontErr != nil {
		return nil, fmt.Errorf("parse embedded mono font: %w", fontErr)
	}
	face, err := opentype.NewFace(parsedMono, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, fmt.Errorf("build font face: %w", err)
	}
	return face, nil
}

func render(lines [][]styledCell, face font.Face, opts Options) (*image.RGBA, error) {
	if len(lines) == 0 {
		lines = [][]styledCell{{}}
	}

	metrics := face.Metrics()
	cellWidth := font.MeasureString(face, "M").Ceil()
	lineHeight := metrics.Height.Ceil()
	if cellWidth <= 0 || lineHeight <= 0 {
		return nil, fmt.Errorf("invalid face metrics")
	}

	cols := 1
	for _, line := range lines {
		if len(line) > cols {
			cols = len(line)
		}
	}
	rows := len(lines)

	width := opts.PaddingX*2 + cols*cellWidth
	height := opts.PaddingY*2 + rows*lineHeight
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), image.NewUniform(defaultTheme.Background), image.Point{}, draw.Src)

	drawer := font.Drawer{
		Dst:  img,
		Face: face,
	}
	ascent := metrics.Ascent.Ceil()

	for row, line := range lines {
		top := opts.PaddingY + row*lineHeight
		baseline := top + ascent
		for col, cell := range line {
			left := opts.PaddingX + col*cellWidth
			fg, bg := cell.style.resolve()
			rect := image.Rect(left, top, left+cellWidth, top+lineHeight)
			draw.Draw(img, rect, image.NewUniform(bg), image.Point{}, draw.Src)
			if cell.r == ' ' {
				continue
			}
			drawer.Src = image.NewUniform(fg)
			drawer.Dot = fixed.P(left, baseline)
			drawer.DrawString(string(cell.r))
		}
	}

	return img, nil
}

func (s style) resolve() (color.Color, color.Color) {
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
		minInt(int(c.R)+int(amount), 255),
		minInt(int(c.G)+int(amount), 255),
		minInt(int(c.B)+int(amount), 255),
	)
}

func parseANSI(text string) ([][]styledCell, error) {
	lines := make([][]styledCell, 1)
	current := defaultStyle()
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
					return nil, fmt.Errorf("unterminated ansi sequence")
				}
				if text[end] == 'm' {
					current = applySGR(current, parseSGRParams(text[i+2:end]))
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
				writeCell(lines, row, col, styledCell{r: ' ', style: current})
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
			if r >= utf8RuneSelf {
				r, size = utf8.DecodeRuneInString(text[i:])
			}
			if r == utf8.RuneError {
				r = '?'
				size = 1
			}
			if !unicode.IsPrint(r) {
				i += size
				continue
			}
			writeCell(lines, row, col, styledCell{r: r, style: current})
			col++
			i += size
		}
	}

	return lines, nil
}

const (
	utf8RuneSelf = 0x80
)

func writeCell(lines [][]styledCell, row int, col int, cell styledCell) {
	line := lines[row]
	for len(line) <= col {
		line = append(line, styledCell{r: ' ', style: defaultStyle()})
	}
	line[col] = cell
	lines[row] = line
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

func applySGR(current style, params []int) style {
	if len(params) == 0 {
		return defaultStyle()
	}
	for i := 0; i < len(params); i++ {
		switch code := params[i]; {
		case code == 0:
			current = defaultStyle()
		case code == 1:
			current.bold = true
		case code == 22:
			current.bold = false
		case code == 7:
			current.reverse = true
		case code == 27:
			current.reverse = false
		case code == 39:
			current.fg = defaultTheme.Foreground
		case code == 49:
			current.bg = defaultTheme.Background
		case code >= 30 && code <= 37:
			current.fg = ansiColor(code - 30)
		case code >= 90 && code <= 97:
			current.fg = ansiColor(code - 90 + 8)
		case code >= 40 && code <= 47:
			current.bg = ansiColor(code - 40)
		case code >= 100 && code <= 107:
			current.bg = ansiColor(code - 100 + 8)
		case code == 38 || code == 48:
			isForeground := code == 38
			parsed, consumed := parseExtendedColor(params[i+1:])
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

func parseExtendedColor(params []int) (color.RGBA, int) {
	if len(params) < 2 {
		return defaultTheme.Foreground, 0
	}
	switch params[0] {
	case 5:
		return ansi256Color(params[1]), 2
	case 2:
		if len(params) < 4 {
			return defaultTheme.Foreground, 0
		}
		return rgba(params[1], params[2], params[3]), 4
	default:
		return defaultTheme.Foreground, 0
	}
}

func defaultStyle() style {
	return style{
		fg: defaultTheme.Foreground,
		bg: defaultTheme.Background,
	}
}

func ansiColor(index int) color.RGBA {
	base := []color.RGBA{
		rgba(40, 44, 52),
		rgba(224, 108, 117),
		rgba(152, 195, 121),
		rgba(229, 192, 123),
		rgba(97, 175, 239),
		rgba(198, 120, 221),
		rgba(86, 182, 194),
		rgba(171, 178, 191),
		rgba(92, 99, 112),
		rgba(248, 113, 113),
		rgba(166, 218, 149),
		rgba(250, 204, 21),
		rgba(96, 165, 250),
		rgba(232, 121, 249),
		rgba(34, 211, 238),
		rgba(255, 255, 255),
	}
	if index < 0 || index >= len(base) {
		return defaultTheme.Foreground
	}
	return base[index]
}

func ansi256Color(index int) color.RGBA {
	if index < 0 {
		index = 0
	}
	if index < 16 {
		return ansiColor(index)
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

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
