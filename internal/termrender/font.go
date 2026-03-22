package termrender

import (
	"fmt"
	"os"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
)

const defaultFontKey = "__embedded_gomono__"

var (
	fontMu      sync.Mutex
	parsedFonts = map[string]*parsedFontEntry{}
)

type parsedFontEntry struct {
	font  *opentype.Font
	err   error
	ready chan struct{}
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
	entry := parsedFonts[key]
	if entry != nil {
		fontMu.Unlock()
		<-entry.ready
		if entry.font != nil {
			return entry.font, nil
		}
		return nil, entry.err
	}
	entry = &parsedFontEntry{ready: make(chan struct{})}
	parsedFonts[key] = entry
	fontMu.Unlock()

	parsedFont, err := parseFont(key, opts.FontFile)
	if err != nil {
		fontMu.Lock()
		entry.err = err
		delete(parsedFonts, key)
		close(entry.ready)
		fontMu.Unlock()
		return nil, err
	}

	fontMu.Lock()
	entry.font = parsedFont
	close(entry.ready)
	fontMu.Unlock()
	return parsedFont, nil
}

func parseFont(key string, fontFile string) (*opentype.Font, error) {
	fontBytes := gomono.TTF
	if key != defaultFontKey {
		var err error
		fontBytes, err = os.ReadFile(fontFile)
		if err != nil {
			return nil, fmt.Errorf("read snapshot font file %s: %w", fontFile, err)
		}
	}

	parsedFont, err := opentype.Parse(fontBytes)
	if err != nil {
		if key == defaultFontKey {
			return nil, fmt.Errorf("parse embedded mono font: %w", err)
		}
		return nil, fmt.Errorf("parse snapshot font file %s: %w", fontFile, err)
	}
	return parsedFont, nil
}
