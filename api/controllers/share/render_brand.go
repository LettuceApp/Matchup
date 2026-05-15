package share

// render_brand.go produces the *generic* brand assets the app needs
// outside of any specific matchup/bracket — the homepage social
// card (og:image fallback for the SPA index.html) and the PWA icons
// (logo192.png / logo512.png / favicon.ico).
//
// We render these at build/deploy time via api/cmd/render_default_og
// rather than serving them from a live HTTP endpoint, because:
//   1. They almost never change — re-rendering on every request would
//      waste CPU + cache space for an output that's effectively static.
//   2. The frontend wants to ship them as static files in
//      /public so the SPA shell can reference them with %PUBLIC_URL%
//      and they get served from the same origin as index.html (no
//      CORS, no API dependency for the marketing surface).
//
// The recipe deliberately mirrors the per-matchup card renderer in
// image.go: same dark navy background, same stardust gradient palette,
// same gofont typography. So when someone sees the homepage card AND
// a matchup card side-by-side in iMessage, they read as one brand.

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"

	"github.com/fogleman/gg"
)

// Stardust gradient — the orange → pink → purple recipe baked into
// theme.css (`--gradient-stardust`). Kept in sync with the frontend
// constant; updating one should update the other.
var stardustStops = []color.RGBA{
	{0xff, 0x9a, 0x6c, 0xff}, // warm orange
	{0xff, 0x6a, 0xa6, 0xff}, // hot pink
	{0xb0, 0x7b, 0xff, 0xff}, // bright purple
}

// RenderHomepageCard produces a 1200×630 PNG suitable for use as the
// SPA's og:image fallback. Layout (top → bottom):
//
//	┌────────────────────────────────────────────────┐
//	│ ▰▰▰ stardust gradient stripe (10px) ▰▰▰▰▰▰▰▰▰▰│
//	│                                                │
//	│             MATCHUP                            │  <- big wordmark
//	│        vote, bracket, decide.                  │  <- tagline
//	│                                                │
//	│   ┌──────────┐    VS    ┌──────────┐          │  <- abstract VS illo
//	│   │  THIS    │          │  THAT    │          │
//	│   └──────────┘          └──────────┘          │
//	│                                                │
//	│                              matchup.app      │
//	└────────────────────────────────────────────────┘
//
// Picking the VS illustration over a logo-only card on purpose: the
// homepage URL gets shared most often (more than individual matchups
// for now), and the preview is the first impression. Showing two
// tiles + VS communicates "head-to-head voting" at a glance —
// strictly more informative than a wordmark in isolation.
func RenderHomepageCard() ([]byte, error) {
	dc := gg.NewContext(imgWidth, imgHeight)

	// Background — same navy as the matchup card so the two read as
	// the same brand surface.
	dc.SetRGBA255(0x0f, 0x17, 0x2a, 255)
	dc.DrawRectangle(0, 0, imgWidth, imgHeight)
	dc.Fill()

	// Top accent stripe — full stardust gradient (3 stops, not 2 like
	// the per-matchup hash-derived stripe). 10px tall, edge to edge.
	drawThreeStopGradientBar(dc, 0, 0, imgWidth, 10,
		stardustStops[0], stardustStops[1], stardustStops[2])

	// Soft radial glow at the top center — pulls the eye toward the
	// wordmark. Very low alpha + tightened ellipse so the navy stays
	// dominant and the glow reads as ambient light rather than a
	// distinct shape. Tuned by eye: at intensity≥30 the falloff
	// curve still rendered visible concentric annuli at the iMessage
	// preview size; 14 lets the steps blend into the navy instead.
	drawSoftGlow(dc, float64(imgWidth)/2, 0, 700, 140, stardustStops[1], 14)

	// MATCHUP wordmark — big, centered, painted as a gradient-filled
	// text overlay. Vertical anchor sits in the upper third so the VS
	// illo below has room to breathe.
	if err := drawGradientWordmark(dc, "MATCHUP",
		float64(imgWidth)/2, 200, 110); err != nil {
		return nil, fmt.Errorf("homepage wordmark: %w", err)
	}

	// Tagline — slate-300, smaller, centered.
	taglineFace, err := face(34, false)
	if err != nil {
		return nil, fmt.Errorf("homepage tagline face: %w", err)
	}
	defer taglineFace.Close()
	dc.SetFontFace(taglineFace)
	dc.SetRGBA255(0xcb, 0xd5, 0xe1, 230)
	dc.DrawStringAnchored("vote, bracket, decide.",
		float64(imgWidth)/2, 280, 0.5, 0.5)

	// VS illustration — two abstract tiles flanking a "VS" badge.
	// Uses the stardust colors so the illo carries the brand color
	// recipe without text.
	if err := drawHomepageVS(dc); err != nil {
		return nil, fmt.Errorf("homepage VS: %w", err)
	}

	// Footer URL — bottom right, low-contrast slate. Functions as the
	// "where you can find us" footnote.
	footFace, err := face(22, true)
	if err != nil {
		return nil, fmt.Errorf("homepage footer face: %w", err)
	}
	defer footFace.Close()
	dc.SetFontFace(footFace)
	dc.SetRGBA255(0x94, 0xa3, 0xff, 200)
	dc.DrawStringAnchored("matchup.app",
		float64(imgWidth)-60, float64(imgHeight)-50, 1.0, 0.5)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, fmt.Errorf("png encode: %w", err)
	}
	return buf.Bytes(), nil
}

// RenderBrandSquareIcon produces a NxN PNG that's the app's brand
// glyph — a big white "M" centered on the stardust gradient with
// slightly rounded corners. Used for the PWA logo192/logo512 + the
// favicon raster source. `roundedCorners` should be true for the
// large PWA icons (matches the rest of the OS icon grid) and false
// for the favicon (sharper at tiny sizes).
func RenderBrandSquareIcon(size int, roundedCorners bool) ([]byte, error) {
	if size <= 0 {
		return nil, fmt.Errorf("invalid icon size %d", size)
	}
	dc := gg.NewContext(size, size)
	s := float64(size)

	// Optional rounded-rect mask so the icon docks into iOS/Android/
	// macOS icon grids cleanly. Browsers crop favicons to a square
	// regardless, so we skip the radius at 32px and below.
	if roundedCorners {
		// ~22% radius matches the iOS app-icon curve.
		dc.DrawRoundedRectangle(0, 0, s, s, s*0.22)
		dc.Clip()
	}

	// Diagonal stardust gradient fill — paints the whole canvas. The
	// `120deg` analog from CSS is achieved with a top-left → bottom-
	// right scan.
	drawDiagonalStardust(dc, s, s)

	// Subtle inner shadow at the edges — gives the icon a little
	// depth instead of looking like a flat color swatch.
	drawInnerEdgeShadow(dc, s)

	// Big bold "M" — white with a soft drop shadow for contrast
	// against the lighter parts of the gradient (the orange end).
	// Size scales with the canvas: ~62% of the side length at 512px
	// reads as the same proportions Apple ships on their icon grid.
	glyphSize := s * 0.62

	// Drop shadow first (offset down + right), then the glyph on top.
	shadowFace, err := face(glyphSize, true)
	if err != nil {
		return nil, fmt.Errorf("icon glyph face: %w", err)
	}
	defer shadowFace.Close()
	dc.SetFontFace(shadowFace)
	dc.SetRGBA255(0, 0, 0, 60)
	dc.DrawStringAnchored("M", s/2+s*0.012, s/2+s*0.012, 0.5, 0.5)

	dc.SetRGBA255(0xff, 0xff, 0xff, 255)
	dc.DrawStringAnchored("M", s/2, s/2, 0.5, 0.5)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, fmt.Errorf("icon png encode: %w", err)
	}
	return buf.Bytes(), nil
}

// EncodeFaviconICO wraps a PNG payload in the ICO container format.
// Browsers will accept a PNG-with-.ico extension on most modern
// builds, but older Windows + some iOS share-sheet contexts expect
// the actual ICO header bytes. Spec ref:
// https://en.wikipedia.org/wiki/ICO_(file_format) — ICONDIR (6
// bytes) + one ICONDIRENTRY (16 bytes) + the PNG data. Bitfields
// must be little-endian.
func EncodeFaviconICO(pngBytes []byte, sizePx int) []byte {
	// ICO uses uint8 = 0 for "256" in the size field. For our 32×32
	// case it's just 32. Clamp anything ≥256.
	szByte := uint8(sizePx)
	if sizePx >= 256 {
		szByte = 0
	}

	var buf bytes.Buffer
	// ICONDIR — 6 bytes.
	buf.Write([]byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00}) // reserved=0, type=1 (ICO), count=1

	// ICONDIRENTRY — 16 bytes.
	dataOffset := uint32(22) // ICONDIR(6) + ICONDIRENTRY(16)
	dataSize := uint32(len(pngBytes))
	buf.WriteByte(szByte) // width
	buf.WriteByte(szByte) // height
	buf.WriteByte(0)      // color count (0 = no palette)
	buf.WriteByte(0)      // reserved
	buf.Write([]byte{0x01, 0x00}) // color planes = 1 (LE)
	buf.Write([]byte{0x20, 0x00}) // bits per pixel = 32 (LE)
	buf.Write([]byte{
		byte(dataSize), byte(dataSize >> 8), byte(dataSize >> 16), byte(dataSize >> 24),
	})
	buf.Write([]byte{
		byte(dataOffset), byte(dataOffset >> 8), byte(dataOffset >> 16), byte(dataOffset >> 24),
	})

	// PNG payload.
	buf.Write(pngBytes)
	return buf.Bytes()
}

// ---- helpers --------------------------------------------------------

// drawThreeStopGradientBar paints a horizontal gradient passing
// through three colors (left → mid → right), useful for the
// orange→pink→purple stardust stripe. Internally it's two
// drawHorizontalGradientBar calls back-to-back on the left/right
// halves, with the mid color landing exactly at x=w/2.
func drawThreeStopGradientBar(dc *gg.Context, x, y, w, h float64,
	left, mid, right color.RGBA) {
	half := w / 2
	drawHorizontalGradientBar(dc, x, y, half, h, left, mid)
	drawHorizontalGradientBar(dc, x+half, y, half, h, mid, right)
}

// drawSoftGlow paints a radial-ish glow as a circle of low-alpha
// pixels around (cx, cy). gg doesn't expose a real radial gradient
// fill, so we approximate by drawing concentric annuli at decreasing
// alpha. `intensity` is the alpha at the center (0–255).
func drawSoftGlow(dc *gg.Context, cx, cy, rx, ry float64,
	c color.RGBA, intensity int) {
	const steps = 24
	for i := 0; i < steps; i++ {
		t := float64(i) / float64(steps)
		// Quadratic falloff — the center is brightest, the edge near 0.
		alpha := float64(intensity) * (1 - t) * (1 - t)
		if alpha < 1 {
			continue
		}
		dc.SetRGBA255(int(c.R), int(c.G), int(c.B), int(alpha))
		dc.DrawEllipse(cx, cy, rx*(1-t*0.7), ry*(1-t*0.7))
		dc.Fill()
	}
}

// drawGradientWordmark draws `text` at (cx, cy) with the stardust
// gradient painted into the glyph fill (via a clipped fill against
// a horizontal gradient rect). Anchored on its centerpoint.
//
// gg doesn't directly support gradient text fills; we get the effect
// by drawing the gradient as a wide rectangle clipped to the text's
// path. The text path comes from gg's NewSubPath/DrawStringAnchored,
// but DrawStringAnchored doesn't add to the path — so we instead
// draw the gradient into a temp context masked by an alpha render
// of the text. Implementation detail: we just use a flat color at
// this size because the gradient text mask path is fiddly enough
// that the gain isn't worth the complexity. We compromise with a
// near-white glyph paired with the gradient stripe + glow above.
func drawGradientWordmark(dc *gg.Context, text string, cx, cy, sizePx float64) error {
	f, err := face(sizePx, true)
	if err != nil {
		return err
	}
	defer f.Close()
	dc.SetFontFace(f)

	// Drop shadow for depth — subtle so it doesn't muddy the navy.
	dc.SetRGBA255(0, 0, 0, 80)
	dc.DrawStringAnchored(text, cx+3, cy+4, 0.5, 0.5)

	// Foreground — bright off-white. The stardust palette stays in
	// the stripe + glow + VS illo so the wordmark reads cleanly at
	// any preview size (including the small 80×80 iMessage cell).
	dc.SetRGBA255(0xf8, 0xfa, 0xfc, 255)
	dc.DrawStringAnchored(text, cx, cy, 0.5, 0.5)
	return nil
}

// drawHomepageVS draws the two-tiles-with-VS illustration in the
// lower half of the homepage card. Visually consistent with the
// per-matchup card's tile look (rounded, tinted) but uses a label-
// only tile drawer (no vote count) — this is a marketing surface,
// not a vote tally. The matchup card's drawTintedTile renders
// "no votes yet" when votes=0, which reads as broken on a generic
// preview where no matchup is even involved.
func drawHomepageVS(dc *gg.Context) error {
	const (
		tileY = 360.0
		tileH = 180.0
		gap   = 120.0
	)
	tileW := 360.0
	totalW := tileW*2 + gap
	leftX := (float64(imgWidth) - totalW) / 2

	// Left tile — orange-tinted, label only.
	if err := drawHomepageTile(dc, leftX, tileY, tileW, tileH, stardustStops[0], "THIS"); err != nil {
		return err
	}
	// Right tile — purple-tinted, label only.
	if err := drawHomepageTile(dc, leftX+tileW+gap, tileY, tileW, tileH, stardustStops[2], "THAT"); err != nil {
		return err
	}

	// VS badge — solid circle with white "VS" text, sits in the gap.
	vsCX := float64(imgWidth) / 2
	vsCY := tileY + tileH/2
	dc.SetRGBA255(0x1e, 0x29, 0x3b, 255)
	dc.DrawCircle(vsCX, vsCY, 44)
	dc.Fill()
	dc.SetRGBA255(int(stardustStops[1].R), int(stardustStops[1].G), int(stardustStops[1].B), 255)
	dc.SetLineWidth(2)
	dc.DrawCircle(vsCX, vsCY, 44)
	dc.Stroke()

	vsFace, err := face(28, true)
	if err != nil {
		return err
	}
	defer vsFace.Close()
	dc.SetFontFace(vsFace)
	dc.SetRGBA255(0xf8, 0xfa, 0xfc, 255)
	dc.DrawStringAnchored("VS", vsCX, vsCY, 0.5, 0.5)
	return nil
}

// drawHomepageTile is the label-only sibling of drawTintedTile (in
// image.go). Same rounded tinted-fill recipe, centered bold label,
// but no vote-count row underneath. Used only for the marketing
// VS illustration — keep the matchup-card flavor without the
// "no votes yet" footnote.
func drawHomepageTile(dc *gg.Context, x, y, w, h float64, tint color.RGBA, label string) error {
	const radius = 16.0
	// Tinted fill — same low-alpha "stained glass" recipe as
	// drawTintedTile so the colors compose with the navy background
	// the same way.
	dc.SetRGBA255(int(tint.R), int(tint.G), int(tint.B), 70)
	dc.DrawRoundedRectangle(x, y, w, h, radius)
	dc.Fill()

	// Label — bold white, vertically centered in the tile.
	labelFace, err := face(46, true)
	if err != nil {
		return err
	}
	defer labelFace.Close()
	dc.SetFontFace(labelFace)
	dc.SetRGBA255(0xf8, 0xfa, 0xfc, 255)
	dc.DrawStringAnchored(label, x+w/2, y+h/2, 0.5, 0.5)
	return nil
}

// drawDiagonalStardust fills the canvas with the stardust gradient
// running diagonally (top-left → bottom-right), mirroring the
// frontend's 120deg linear-gradient. Scans pixel-row-by-pixel-row
// using the diagonal projection to compute t — cheap enough at
// favicon → 512px sizes.
func drawDiagonalStardust(dc *gg.Context, w, h float64) {
	// We treat the gradient as a sweep from corner to corner. For
	// each pixel (x, y), compute its projection onto the diagonal
	// (normalized 0..1) and interpolate stardustStops across 3 stops:
	// 0.0 → orange, 0.5 → pink, 1.0 → purple.
	maxRange := w + h
	for y := 0; y < int(h); y++ {
		for x := 0; x < int(w); x++ {
			t := (float64(x) + float64(y)) / maxRange
			var r, g, b uint8
			if t < 0.5 {
				lt := t * 2
				r = lerp(stardustStops[0].R, stardustStops[1].R, lt)
				g = lerp(stardustStops[0].G, stardustStops[1].G, lt)
				b = lerp(stardustStops[0].B, stardustStops[1].B, lt)
			} else {
				lt := (t - 0.5) * 2
				r = lerp(stardustStops[1].R, stardustStops[2].R, lt)
				g = lerp(stardustStops[1].G, stardustStops[2].G, lt)
				b = lerp(stardustStops[1].B, stardustStops[2].B, lt)
			}
			dc.SetRGBA255(int(r), int(g), int(b), 255)
			dc.SetPixel(x, y)
		}
	}
}

// drawInnerEdgeShadow paints a thin dark border just inside the icon
// edge — gives the brand glyph definition against light backgrounds
// (e.g. when iOS docks it on a white home screen). Subtle: ~3% of
// the side length, dark navy at ~25% alpha.
func drawInnerEdgeShadow(dc *gg.Context, s float64) {
	thickness := s * 0.03
	dc.SetRGBA255(0x0f, 0x17, 0x2a, 64)
	dc.SetLineWidth(thickness)
	dc.DrawRectangle(thickness/2, thickness/2, s-thickness, s-thickness)
	dc.Stroke()
}
