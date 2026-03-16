package termrender

import (
	"bytes"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
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
