package termrender

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strings"
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
	ThemeDark  = "dark"
	ThemeLight = "light"
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
