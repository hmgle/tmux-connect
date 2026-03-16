package termrender

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
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
	FontSize  float64
	FontFile  string
	ThemeName string
	PaddingX  int
	PaddingY  int
}

type Theme struct {
	Name       string
	Foreground color.RGBA
	Background color.RGBA
	Palette    [16]color.RGBA
}

const (
	ThemeDark      = "dark"
	ThemeLight     = "light"
	defaultFontKey = "__embedded_gomono__"
)

var themes = map[string]Theme{
	ThemeDark: {
		Name:       ThemeDark,
		Foreground: rgba(217, 224, 238),
		Background: rgba(17, 24, 39),
		Palette: [16]color.RGBA{
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
		},
	},
	ThemeLight: {
		Name:       ThemeLight,
		Foreground: rgba(31, 41, 55),
		Background: rgba(248, 250, 252),
		Palette: [16]color.RGBA{
			rgba(100, 116, 139),
			rgba(185, 28, 28),
			rgba(22, 101, 52),
			rgba(161, 98, 7),
			rgba(29, 78, 216),
			rgba(147, 51, 234),
			rgba(13, 148, 136),
			rgba(71, 85, 105),
			rgba(148, 163, 184),
			rgba(220, 38, 38),
			rgba(21, 128, 61),
			rgba(202, 138, 4),
			rgba(37, 99, 235),
			rgba(168, 85, 247),
			rgba(15, 118, 110),
			rgba(15, 23, 42),
		},
	},
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
	fontMu      sync.Mutex
	parsedFonts = map[string]*opentype.Font{}
)

func RenderPNG(text string, opts Options) ([]byte, error) {
	opts, theme, err := prepareOptions(opts)
	if err != nil {
		return nil, err
	}
	lines, err := parseANSI(text, theme)
	if err != nil {
		return nil, err
	}
	face, err := loadFace(opts)
	if err != nil {
		return nil, err
	}
	defer face.Close()

	img, err := render(lines, face, theme, opts)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}

func ValidateOptions(opts Options) error {
	_, _, err := prepareOptions(opts)
	return err
}

func normalizeOptions(opts Options) Options {
	opts.ThemeName = normalizeThemeName(opts.ThemeName)
	if opts.FontSize <= 0 {
		opts.FontSize = 14
	}
	opts.FontFile = strings.TrimSpace(opts.FontFile)
	if opts.PaddingX <= 0 {
		opts.PaddingX = 14
	}
	if opts.PaddingY <= 0 {
		opts.PaddingY = 12
	}
	return opts
}

func normalizeThemeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ThemeDark
	}
	return name
}

func IsSupportedThemeName(name string) bool {
	_, ok := themes[normalizeThemeName(name)]
	return ok
}

func prepareOptions(opts Options) (Options, Theme, error) {
	opts = normalizeOptions(opts)
	theme, ok := themes[opts.ThemeName]
	if !ok {
		return Options{}, Theme{}, fmt.Errorf("unsupported snapshot theme %q", opts.ThemeName)
	}
	if opts.FontSize <= 0 {
		return Options{}, Theme{}, fmt.Errorf("snapshot font size must be > 0")
	}
	if opts.FontFile != "" {
		ext := strings.ToLower(filepath.Ext(opts.FontFile))
		if ext != ".ttf" && ext != ".otf" {
			return Options{}, Theme{}, fmt.Errorf("snapshot font file must be .ttf or .otf")
		}
	}
	if _, err := loadParsedFont(opts); err != nil {
		return Options{}, Theme{}, err
	}
	return opts, theme, nil
}

func loadFace(opts Options) (font.Face, error) {
	parsedFont, err := loadParsedFont(opts)
	if err != nil {
		return nil, err
	}
	face, err := opentype.NewFace(parsedFont, &opentype.FaceOptions{
		Size:    opts.FontSize,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, fmt.Errorf("build font face: %w", err)
	}
	return face, nil
}

func loadParsedFont(opts Options) (*opentype.Font, error) {
	key := defaultFontKey
	if opts.FontFile != "" {
		key = opts.FontFile
	}

	fontMu.Lock()
	parsedFont := parsedFonts[key]
	fontMu.Unlock()
	if parsedFont != nil {
		return parsedFont, nil
	}

	var fontBytes []byte
	var err error
	if key == defaultFontKey {
		fontBytes = gomono.TTF
	} else {
		fontBytes, err = os.ReadFile(opts.FontFile)
		if err != nil {
			return nil, fmt.Errorf("read snapshot font file %s: %w", opts.FontFile, err)
		}
	}

	parsedFont, err = opentype.Parse(fontBytes)
	if err != nil {
		if key == defaultFontKey {
			return nil, fmt.Errorf("parse embedded mono font: %w", err)
		}
		return nil, fmt.Errorf("parse snapshot font file %s: %w", opts.FontFile, err)
	}

	fontMu.Lock()
	defer fontMu.Unlock()
	if cached := parsedFonts[key]; cached != nil {
		return cached, nil
	}
	parsedFonts[key] = parsedFont
	return parsedFont, nil
}

func render(lines [][]styledCell, face font.Face, theme Theme, opts Options) (*image.RGBA, error) {
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
	draw.Draw(img, img.Bounds(), image.NewUniform(theme.Background), image.Point{}, draw.Src)

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
					return nil, fmt.Errorf("unterminated ansi sequence")
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
			writeCell(lines, row, col, styledCell{r: r, style: current}, theme)
			col++
			i += size
		}
	}

	return lines, nil
}

const (
	utf8RuneSelf = 0x80
)

func writeCell(lines [][]styledCell, row int, col int, cell styledCell, theme Theme) {
	line := lines[row]
	for len(line) <= col {
		line = append(line, styledCell{r: ' ', style: defaultStyle(theme)})
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

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
