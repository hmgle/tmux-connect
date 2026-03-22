package termrender

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

const (
	maxRenderDimension = 10_000
	maxRenderPixels    = 40_000_000
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

func render(lines [][]styledCell, face font.Face, theme Theme, opts Options) (*image.RGBA, error) {
	if len(lines) == 0 {
		lines = [][]styledCell{{blankCell(theme)}}
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
	if width > maxRenderDimension || height > maxRenderDimension {
		return nil, fmt.Errorf("snapshot image too large: %dx%d exceeds %dpx limit", width, height, maxRenderDimension)
	}
	if width*height > maxRenderPixels {
		return nil, fmt.Errorf("snapshot image too large: %dx%d exceeds pixel budget", width, height)
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	uniforms := map[color.RGBA]*image.Uniform{}
	uniformFor := func(c color.RGBA) *image.Uniform {
		if uniform := uniforms[c]; uniform != nil {
			return uniform
		}
		uniform := image.NewUniform(c)
		uniforms[c] = uniform
		return uniform
	}
	draw.Draw(img, img.Bounds(), uniformFor(theme.Background), image.Point{}, draw.Src)

	drawer := font.Drawer{
		Dst:  img,
		Face: face,
	}
	ascent := metrics.Ascent.Ceil()

	for row, line := range lines {
		top := opts.PaddingY + row*lineHeight
		baseline := top + ascent
		for col, cell := range line {
			if cell.continuation {
				continue
			}

			span := cell.span
			if span <= 0 {
				span = 1
			}
			left := opts.PaddingX + col*cellWidth
			fg, bg := cell.style.resolve()
			rect := image.Rect(left, top, left+span*cellWidth, top+lineHeight)
			draw.Draw(img, rect, uniformFor(bg), image.Point{}, draw.Src)
			if cell.r == ' ' || cell.r == 0 {
				continue
			}
			drawer.Src = uniformFor(fg)
			drawer.Dot = fixed.P(left, baseline)
			drawer.DrawString(string(cell.r))
		}
	}

	return img, nil
}
