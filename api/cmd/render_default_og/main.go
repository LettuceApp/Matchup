// render_default_og emits the static brand asset set that the SPA
// shell references in its index.html: the 1200×630 og:image card
// that link unfurlers (iMessage, Twitter, Slack, Facebook) request
// when someone shares the bare homepage URL, plus the PWA icon
// trio (logo192.png, logo512.png, favicon.ico).
//
// Usage:
//
//	go run ./cmd/render_default_og
//
// All outputs land in ../frontend/public/ relative to the api/
// directory. Re-run any time the brand recipe in share/render_brand.go
// changes; commit the resulting PNG/ICO blobs so prod CDN serves
// them without re-rendering.
//
// Why not serve dynamically from a /og/default.png endpoint? The
// SPA's index.html lives on the same CDN as the rest of /public — we
// want the og:image to live next to it so the marketing surface
// has no runtime API dependency. Re-rendering the homepage card on
// every crawl would also burn CPU for an output that effectively
// never changes.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"Matchup/controllers/share"
)

// Where to drop the generated assets. Paths are relative to the api/
// working directory the cmd was invoked from (i.e. `go run` cwd).
var outputs = []struct {
	path string
	make func() ([]byte, error)
}{
	{
		path: "../frontend/public/og-default.png",
		make: share.RenderHomepageCard,
	},
	{
		path: "../frontend/public/logo512.png",
		make: func() ([]byte, error) {
			return share.RenderBrandSquareIcon(512, true)
		},
	},
	{
		path: "../frontend/public/logo192.png",
		make: func() ([]byte, error) {
			return share.RenderBrandSquareIcon(192, true)
		},
	},
	{
		// favicon-32.png: a sharp 32×32 source PNG. Browsers that
		// hit /favicon.ico still get the ICO wrapper below; we keep
		// this PNG around for the `<link rel="icon" type="image/png">`
		// fallback in index.html.
		path: "../frontend/public/favicon-32.png",
		make: func() ([]byte, error) {
			return share.RenderBrandSquareIcon(32, false)
		},
	},
}

func main() {
	for _, o := range outputs {
		data, err := o.make()
		if err != nil {
			log.Fatalf("render %s: %v", o.path, err)
		}
		if err := writeFile(o.path, data); err != nil {
			log.Fatalf("write %s: %v", o.path, err)
		}
		fmt.Printf("wrote %s (%d bytes)\n", o.path, len(data))
	}

	// Favicon.ico — same 32×32 PNG payload, wrapped in the ICO
	// container header so legacy tooling (Windows Start menu pins,
	// some RSS readers) reads it correctly.
	favPNG, err := share.RenderBrandSquareIcon(32, false)
	if err != nil {
		log.Fatalf("render favicon source: %v", err)
	}
	ico := share.EncodeFaviconICO(favPNG, 32)
	if err := writeFile("../frontend/public/favicon.ico", ico); err != nil {
		log.Fatalf("write favicon.ico: %v", err)
	}
	fmt.Printf("wrote ../frontend/public/favicon.ico (%d bytes)\n", len(ico))
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
