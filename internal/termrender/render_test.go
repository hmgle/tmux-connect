package termrender

import (
	"bytes"
	"image/png"
	"testing"
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
