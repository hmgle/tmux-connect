package termrender

import (
	"bytes"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/gomono"
)

func TestRenderPNG(t *testing.T) {
	t.Parallel()

	data, err := RenderPNG("\x1b[31merror\x1b[0m\nplain", Options{})
	if err != nil {
		t.Fatalf("RenderPNG() error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("RenderPNG() returned empty data")
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("png decode error = %v", err)
	}
	if img.Bounds().Dx() <= 0 || img.Bounds().Dy() <= 0 {
		t.Fatalf("invalid bounds %v", img.Bounds())
	}
}

func TestRenderPNGSupportsLightTheme(t *testing.T) {
	t.Parallel()

	data, err := RenderPNG("plain", Options{ThemeName: ThemeLight})
	if err != nil {
		t.Fatalf("RenderPNG() error = %v", err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("png decode error = %v", err)
	}
	got := color.RGBAModel.Convert(img.At(0, 0)).(color.RGBA)
	want := themes[ThemeLight].Background
	if got != want {
		t.Fatalf("background = %#v, want %#v", got, want)
	}
}

func TestRenderPNGHonorsFontSize(t *testing.T) {
	t.Parallel()

	small, err := RenderPNG("plain", Options{FontSize: 12})
	if err != nil {
		t.Fatalf("RenderPNG(small) error = %v", err)
	}
	large, err := RenderPNG("plain", Options{FontSize: 24})
	if err != nil {
		t.Fatalf("RenderPNG(large) error = %v", err)
	}
	smallImg, err := png.Decode(bytes.NewReader(small))
	if err != nil {
		t.Fatalf("png decode small error = %v", err)
	}
	largeImg, err := png.Decode(bytes.NewReader(large))
	if err != nil {
		t.Fatalf("png decode large error = %v", err)
	}
	if largeImg.Bounds().Dy() <= smallImg.Bounds().Dy() {
		t.Fatalf("large height = %d, want > %d", largeImg.Bounds().Dy(), smallImg.Bounds().Dy())
	}
	if largeImg.Bounds().Dx() <= smallImg.Bounds().Dx() {
		t.Fatalf("large width = %d, want > %d", largeImg.Bounds().Dx(), smallImg.Bounds().Dx())
	}
}

func TestRenderPNGSupportsFontFile(t *testing.T) {
	t.Parallel()

	fontPath := filepath.Join(t.TempDir(), "gomono.ttf")
	if err := os.WriteFile(fontPath, gomono.TTF, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	data, err := RenderPNG("plain", Options{FontFile: fontPath})
	if err != nil {
		t.Fatalf("RenderPNG() error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("RenderPNG() returned empty data")
	}
}

func TestRenderPNGRejectsInvalidFontFile(t *testing.T) {
	t.Parallel()

	fontPath := filepath.Join(t.TempDir(), "broken.ttf")
	if err := os.WriteFile(fontPath, []byte("not a font"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := RenderPNG("plain", Options{FontFile: fontPath}); err == nil {
		t.Fatal("RenderPNG() error = nil, want error")
	}
}

func TestParseANSIIgnoresTruncatedSequence(t *testing.T) {
	t.Parallel()

	lines, err := parseANSI("ok\x1b[38;2;255;0", themes[ThemeDark])
	if err != nil {
		t.Fatalf("parseANSI() error = %v", err)
	}
	if got := visibleText(lines[0]); got != "ok" {
		t.Fatalf("visible text = %q, want %q", got, "ok")
	}
}

func TestParseANSIWideRuneUsesTwoCells(t *testing.T) {
	t.Parallel()

	lines, err := parseANSI("你a", themes[ThemeDark])
	if err != nil {
		t.Fatalf("parseANSI() error = %v", err)
	}
	line := lines[0]
	if len(line) != 3 {
		t.Fatalf("len(line) = %d, want 3", len(line))
	}
	if got := line[0].r; got != '你' {
		t.Fatalf("line[0].r = %q, want %q", got, '你')
	}
	if got := line[0].span; got != 2 {
		t.Fatalf("line[0].span = %d, want 2", got)
	}
	if !line[1].continuation {
		t.Fatal("line[1] should be continuation cell")
	}
	if got := line[2].r; got != 'a' {
		t.Fatalf("line[2].r = %q, want %q", got, 'a')
	}
}

func TestParseANSIExtendedColors(t *testing.T) {
	t.Parallel()

	lines, err := parseANSI("\x1b[38;5;196mX\x1b[48;2;1;2;3mY", themes[ThemeDark])
	if err != nil {
		t.Fatalf("parseANSI() error = %v", err)
	}
	line := lines[0]
	if got, want := line[0].style.fg, ansi256Color(themes[ThemeDark], 196); got != want {
		t.Fatalf("line[0].style.fg = %#v, want %#v", got, want)
	}
	if got, want := line[1].style.bg, rgba(1, 2, 3); got != want {
		t.Fatalf("line[1].style.bg = %#v, want %#v", got, want)
	}
}

func TestRenderPNGRejectsOversizedImage(t *testing.T) {
	t.Parallel()

	_, err := RenderPNG(strings.Repeat("x\n", 700), Options{})
	if err == nil {
		t.Fatal("RenderPNG() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Fatalf("RenderPNG() error = %v, want too large error", err)
	}
}

func visibleText(line []styledCell) string {
	var builder strings.Builder
	for _, cell := range line {
		if cell.continuation || cell.r == 0 {
			continue
		}
		builder.WriteRune(cell.r)
	}
	return builder.String()
}
