package share

import (
	"fmt"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

// Fonts ships inside the binary via `golang.org/x/image/font/gofont` —
// Go's official designed-for-the-web font family, shipped with the Go
// image package. Zero new embedded bytes, zero network fetch, and
// licensed BSD.
//
// This is a v1 choice. To upgrade typography later (better kerning,
// variable axes, wider script coverage), swap the gofont TTFs for
// Inter / Noto / etc. via //go:embed in this file. No other callers
// need to change.

// faceCache holds parsed opentype.Font instances keyed by (weight,
// pixel size). Parsing is surprisingly expensive and every preview
// goes through here; we don't want to repay that cost on every render.
var faceCache = struct {
	regular *opentype.Font
	bold    *opentype.Font
}{}

func init() {
	// Panic on init failure: we can't render without fonts, and the
	// gofont bytes are baked into the dep — if this fails, something
	// is very wrong with the build.
	f, err := opentype.Parse(goregular.TTF)
	if err != nil {
		panic(fmt.Sprintf("share/fonts: parse goregular: %v", err))
	}
	faceCache.regular = f

	b, err := opentype.Parse(gobold.TTF)
	if err != nil {
		panic(fmt.Sprintf("share/fonts: parse gobold: %v", err))
	}
	faceCache.bold = b
}

// face returns a fresh font.Face at the requested pixel size and weight.
// Callers should Close() when done — opentype faces hold buffers.
func face(sizePx float64, bold bool) (font.Face, error) {
	src := faceCache.regular
	if bold {
		src = faceCache.bold
	}
	return opentype.NewFace(src, &opentype.FaceOptions{
		Size:    sizePx,
		DPI:     72, // pixels-per-inch; with Size in px this just means 1:1
		Hinting: font.HintingFull,
	})
}
