package share

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"strings"

	"github.com/fogleman/gg"
)

// Standard OG image size. Every major platform (X, Facebook, LinkedIn,
// Discord, Slack, iMessage via LinkPresentation) expects 1200×630 or
// will crop to it. Going larger is wasted bytes; smaller gets rejected.
const (
	imgWidth  = 1200
	imgHeight = 630
)

// ImageInput is everything the renderer needs to produce a preview.
// Kept narrow — the handler unwraps a full DB record into this shape,
// the renderer stays decoupled from the model layer.
type ImageInput struct {
	Kind       ContentKind
	Title      string
	ItemA      string // matchup only — leave empty for bracket
	ItemB      string // matchup only
	VotesA     int64  // matchup only
	VotesB     int64  // matchup only
	AuthorName string
	Likes      int64  // engagement line — sum across the item
	Comments   int64  // engagement line — sum across the item
	ShareURL   string // full https://... URL, kept for forward-compat (no QR in v2 card)
	// BackgroundImage — when non-nil, drawn as the card's full-bleed
	// background BEFORE the navy fill + accent stripe. Used for
	// matchups that have an uploaded image (matchups.image_path);
	// the existing card content overlays the image with a dark scrim
	// so text + tiles stay legible. Nil falls back to the solid navy
	// background, which is the original look. The handler is
	// responsible for fetching + decoding the image; this struct
	// keeps the renderer's signature decoupled from net I/O.
	BackgroundImage image.Image
}

// Gradient palette — same visual identity as the home-feed cards so a
// matchup's preview matches its card on the homepage. Keep this in sync
// with frontend/src/components/HomeCard.js::GRADIENTS.
var gradients = [][]color.RGBA{
	{{0x66, 0x7e, 0xea, 0xff}, {0x76, 0x4b, 0xa2, 0xff}},
	{{0xf0, 0x93, 0xfb, 0xff}, {0xf5, 0x57, 0x6c, 0xff}},
	{{0x4f, 0xac, 0xfe, 0xff}, {0x00, 0xf2, 0xfe, 0xff}},
	{{0x43, 0xe9, 0x7b, 0xff}, {0x38, 0xf9, 0xd7, 0xff}},
	{{0xfa, 0x70, 0x9a, 0xff}, {0xfe, 0xe1, 0x40, 0xff}},
	{{0xa1, 0x8c, 0xd1, 0xff}, {0xfb, 0xc2, 0xeb, 0xff}},
	{{0xfc, 0xcb, 0x90, 0xff}, {0xd5, 0x7e, 0xeb, 0xff}},
	{{0xa1, 0xc4, 0xfd, 0xff}, {0xc2, 0xe9, 0xfb, 0xff}},
}

// gradientFor maps a title to a stable gradient index. Using a hash
// means the same title always gets the same colors (visual identity
// stability across renders) without us having to store anything.
func gradientFor(title string) []color.RGBA {
	if title == "" {
		return gradients[0]
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(title))
	return gradients[h.Sum32()%uint32(len(gradients))]
}

// Render produces a 1200×630 PNG from the given input. Returns the raw
// PNG bytes ready to write to the response body or a cache.
//
// Design: "X-style" dark card.
//   - Solid dark-navy background (matches the frontend app shell).
//   - Accent stripe across the top using the matchup's hash-derived
//     gradient — gives each preview its own color identity without
//     drowning the text behind the color.
//   - Big auto-fit title at top-left.
//   - For matchups: two contender tiles side-by-side with accent
//     tinting; thin leader bar below them.
//   - For brackets: a single subtitle line with size + round.
//   - Bottom row: colored avatar circle with the author's initial,
//     the @username, and a middle-dot engagement line (votes · likes
//     · comments).
func Render(in ImageInput) ([]byte, error) {
	dc := gg.NewContext(imgWidth, imgHeight)

	// --- Background: solid dark navy, matches site shell. Always
	//     painted first so a partially-failed background-image draw
	//     never leaves transparent pixels showing through.
	dc.SetRGBA255(0x0f, 0x17, 0x2a, 255)
	dc.DrawRectangle(0, 0, imgWidth, imgHeight)
	dc.Fill()

	// --- Optional uploaded matchup image as the full-bleed background.
	//     Scaled object-fit-style to cover the canvas while preserving
	//     aspect; a dark scrim over the top keeps the title + tiles
	//     legible regardless of what the image looks like.
	if in.BackgroundImage != nil {
		drawCoverImage(dc, in.BackgroundImage)
		// 65% navy scrim — readable text floor without losing the
		// underlying image entirely. Tuned by eye against light +
		// dark uploaded photos; lower alphas blew out the title on
		// pale images, higher alphas effectively erased the image.
		dc.SetRGBA255(0x0f, 0x17, 0x2a, 165)
		dc.DrawRectangle(0, 0, imgWidth, imgHeight)
		dc.Fill()
	}

	// --- Accent stripe: 10px bar at the top, using the matchup's
	//     hash-derived gradient colors for color-identity continuity
	//     with the home-feed card. Keeps the text area undisturbed.
	accent := gradientFor(in.Title)
	drawHorizontalGradientBar(dc, 0, 0, imgWidth, 10, accent[0], accent[1])

	// --- Brand + kind pill row -----------------------------------------
	if err := drawText(dc, "MATCHUP.APP", 60, 78, 22, true, color.RGBA{148, 163, 255, 255}); err != nil {
		return nil, fmt.Errorf("wordmark: %w", err)
	}
	kindLabel := "MATCHUP"
	if in.Kind == KindBracket {
		kindLabel = "BRACKET"
	}
	if err := drawKindPill(dc, kindLabel); err != nil {
		return nil, fmt.Errorf("kind pill: %w", err)
	}

	// --- Title: big, white, bold, auto-fit two lines -------------------
	if err := drawTitle(dc, in.Title); err != nil {
		return nil, fmt.Errorf("title: %w", err)
	}

	// --- Middle content: matchup contenders or bracket subtitle --------
	switch in.Kind {
	case KindMatchup:
		if err := drawContenderTiles(dc, in, accent); err != nil {
			return nil, fmt.Errorf("contenders: %w", err)
		}
	case KindBracket:
		if err := drawBracketStats(dc, in); err != nil {
			return nil, fmt.Errorf("bracket stats: %w", err)
		}
	}

	// --- Bottom row: avatar + author + engagement line -----------------
	if err := drawAuthorAndEngagement(dc, in, accent); err != nil {
		return nil, fmt.Errorf("engagement row: %w", err)
	}

	// Encode. Default compression keeps output ~80–250KB, well under
	// the iMessage 1MB threshold and fast enough to cache.
	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, fmt.Errorf("png encode: %w", err)
	}
	return buf.Bytes(), nil
}

// drawHorizontalGradientBar paints a thin horizontal gradient between
// two colors, used as the accent stripe at the top of the card. Scans
// vertical columns (one draw per pixel of width) because the bar is
// narrow on the y-axis and long on the x.
// drawCoverImage paints `img` onto the canvas with object-fit:cover
// semantics — scaled uniformly to fill 1200×630 with cropping rather
// than letterboxing. Used by Render() when ImageInput.BackgroundImage
// is set. We compute the larger of the width-scale and height-scale,
// then center the image on the canvas so the cropped pixels come off
// the longer dimension equally on both sides.
func drawCoverImage(dc *gg.Context, img image.Image) {
	bounds := img.Bounds()
	srcW := float64(bounds.Dx())
	srcH := float64(bounds.Dy())
	if srcW <= 0 || srcH <= 0 {
		return
	}
	// object-fit: cover scale — whichever dimension needs MORE
	// magnification to reach the canvas drives the scale, so the
	// other dimension overflows (and gets cropped by the canvas).
	scale := imgWidth / srcW
	if heightScale := imgHeight / srcH; heightScale > scale {
		scale = heightScale
	}
	scaledW := srcW * scale
	scaledH := srcH * scale
	dx := (imgWidth - scaledW) / 2
	dy := (imgHeight - scaledH) / 2
	// gg's ScaleAbout/Translate dance is the documented way to draw
	// a scaled+positioned image. We isolate the transform in a
	// Push/Pop so the surrounding draws (gradient stripe, text, etc.)
	// keep the identity matrix.
	dc.Push()
	dc.Translate(dx, dy)
	dc.Scale(scale, scale)
	dc.DrawImage(img, 0, 0)
	dc.Pop()
}

func drawHorizontalGradientBar(dc *gg.Context, x, y, w, h float64, left, right color.RGBA) {
	for i := 0; i < int(w); i++ {
		t := float64(i) / w
		r := lerp(left.R, right.R, t)
		g := lerp(left.G, right.G, t)
		b := lerp(left.B, right.B, t)
		dc.SetRGBA255(int(r), int(g), int(b), 255)
		dc.DrawRectangle(x+float64(i), y, 1, h)
		dc.Fill()
	}
}

func lerp(a, b uint8, t float64) uint8 {
	return uint8(float64(a)*(1-t) + float64(b)*t)
}

// drawKindPill renders the "MATCHUP" / "BRACKET" label pill top-right
// of the accent stripe. Subtle tinted fill + colored border keeps it
// readable without grabbing focus from the title.
func drawKindPill(dc *gg.Context, label string) error {
	const (
		pw, ph = 160.0, 42.0
		px, py = float64(imgWidth) - 60 - pw, 60.0
		radius = 21.0
	)
	// Subtle tinted background.
	dc.SetRGBA255(0x1e, 0x29, 0x3b, 255)
	dc.DrawRoundedRectangle(px, py, pw, ph, radius)
	dc.Fill()
	// 1px "border" done as a slightly larger rounded rect first, then
	// the fill on top — gg doesn't expose a stroke width for rounded
	// rects that looks right at tiny thicknesses.
	dc.SetRGBA255(0x94, 0xa3, 0xff, 110)
	dc.SetLineWidth(1.5)
	dc.DrawRoundedRectangle(px+0.5, py+0.5, pw-1, ph-1, radius)
	dc.Stroke()

	face, err := face(18, true)
	if err != nil {
		return err
	}
	defer face.Close()
	dc.SetFontFace(face)
	dc.SetRGBA255(0x94, 0xa3, 0xff, 255)
	dc.DrawStringAnchored(label, px+pw/2, py+ph/2, 0.5, 0.5)
	return nil
}

// drawTitle renders the matchup title at an auto-fit size, left-aligned
// under the top brand row. Up to 2 lines. Anchored to a fixed top-y so
// the contender row below never shifts when title length changes.
func drawTitle(dc *gg.Context, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Untitled"
	}

	const (
		maxWidth    = float64(imgWidth) - 120 // 60px padding each side
		baselineTop = 190.0                   // top of the first line
	)

	// Auto-fit loop: start at 76px and step down until the title fits
	// on at most 2 lines. The lower bound is still generous enough
	// that long titles remain legible at iMessage preview size.
	sizes := []float64{76, 66, 58, 50, 44}
	var (
		chosenSize  float64
		chosenLines []string
	)
	for _, size := range sizes {
		lines, ok := wrapText(title, size, maxWidth, 2)
		if ok {
			chosenSize = size
			chosenLines = lines
			break
		}
	}
	if chosenLines == nil {
		chosenSize = 44
		lines, _ := wrapText(title, chosenSize, maxWidth, 2)
		if len(lines) == 0 {
			chosenLines = []string{title}
		} else {
			chosenLines = lines
		}
	}

	f, err := face(chosenSize, true)
	if err != nil {
		return err
	}
	defer f.Close()
	dc.SetFontFace(f)
	dc.SetRGBA255(0xf8, 0xfa, 0xfc, 255)

	lineHeight := chosenSize * 1.15
	for i, line := range chosenLines {
		dc.DrawString(line, 60, baselineTop+float64(i)*lineHeight)
	}
	return nil
}

// wrapText fits `text` into at most `maxLines` of width `maxW` at the
// given font size. Returns (lines, true) on success and (_, false) when
// even aggressive wrapping can't fit it all.
func wrapText(text string, size, maxW float64, maxLines int) ([]string, bool) {
	f, err := face(size, true)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	tmp := gg.NewContext(1, 1)
	tmp.SetFontFace(f)

	measure := func(s string) float64 {
		w, _ := tmp.MeasureString(s)
		return w
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}, true
	}

	var lines []string
	current := words[0]
	for _, w := range words[1:] {
		try := current + " " + w
		if measure(try) <= maxW {
			current = try
			continue
		}
		lines = append(lines, current)
		current = w
		if len(lines) >= maxLines {
			return nil, false
		}
	}
	lines = append(lines, current)
	if len(lines) > maxLines {
		return nil, false
	}
	// Confirm every line fits — catches degenerate "one word too long"
	// cases that slip past the running logic above.
	for _, ln := range lines {
		if measure(ln) > maxW {
			return nil, false
		}
	}
	return lines, true
}

// drawContenderTiles renders two side-by-side cards — one per
// contender — with a narrow "VS" divider between them. Each tile
// carries a tinted fill (using the matchup's accent color at low
// alpha) plus vote count if any. A thin leader bar spans the two
// tiles at the bottom so the lead is readable at a glance.
func drawContenderTiles(dc *gg.Context, in ImageInput, accent []color.RGBA) error {
	a := strings.TrimSpace(in.ItemA)
	b := strings.TrimSpace(in.ItemB)
	if a == "" && b == "" {
		return nil
	}
	if a == "" {
		a = "?"
	}
	if b == "" {
		b = "?"
	}

	const (
		tileTop    = 340.0
		tileBottom = 470.0
		tileH      = tileBottom - tileTop
		margin     = 60.0
		gap        = 24.0
	)
	tileW := (float64(imgWidth) - 2*margin - gap) / 2

	// Two tinted tiles — use the two gradient anchor colors from the
	// palette so left vs right have distinct identity. Keep the alpha
	// low so white text stays legible.
	leftFill := accent[0]
	rightFill := accent[1]

	drawTintedTile(dc, margin, tileTop, tileW, tileH, leftFill, a, in.VotesA)
	drawTintedTile(dc, margin+tileW+gap, tileTop, tileW, tileH, rightFill, b, in.VotesB)

	// "VS" glyph centered in the gap between the tiles.
	vsFace, err := face(28, true)
	if err != nil {
		return err
	}
	defer vsFace.Close()
	dc.SetFontFace(vsFace)
	dc.SetRGBA255(0x94, 0xa3, 0xff, 255)
	dc.DrawStringAnchored("VS", float64(imgWidth)/2, tileTop+tileH/2, 0.5, 0.5)

	// Leader bar, only if there have been any votes. Spans the width
	// of both tiles + gap so the left/right split lines up visually.
	total := in.VotesA + in.VotesB
	if total > 0 {
		const (
			barY   = tileBottom + 22
			barH   = 6.0
			radius = 3.0
		)
		barX := margin
		barW := float64(imgWidth) - 2*margin
		// Track.
		dc.SetRGBA255(0x1e, 0x29, 0x3b, 255)
		dc.DrawRoundedRectangle(barX, barY, barW, barH, radius)
		dc.Fill()
		// Left fill — proportional to item A's share.
		aShare := float64(in.VotesA) / float64(total)
		if aShare > 0.01 {
			dc.SetRGBA255(int(leftFill.R), int(leftFill.G), int(leftFill.B), 255)
			dc.DrawRoundedRectangle(barX, barY, barW*aShare, barH, radius)
			dc.Fill()
		}
	}
	return nil
}

// drawTintedTile paints a single contender tile: rounded background,
// bold name centered, big vote count below. Name is auto-truncated to
// keep a single line even for verbose contenders.
func drawTintedTile(dc *gg.Context, x, y, w, h float64, tint color.RGBA, name string, votes int64) {
	const radius = 16.0

	// Fill: tint at low alpha over the dark navy for a "stained glass"
	// feel — clearly colored but text stays legible.
	dc.SetRGBA255(int(tint.R), int(tint.G), int(tint.B), 70)
	dc.DrawRoundedRectangle(x, y, w, h, radius)
	dc.Fill()

	// Name — bold, white.
	nameFace, err := face(38, true)
	if err == nil {
		defer nameFace.Close()
		dc.SetFontFace(nameFace)
		dc.SetRGBA255(0xf8, 0xfa, 0xfc, 255)
		displayName := truncate(name, 24)
		// Shrink if the exact-truncated name still overflows.
		if tw, _ := dc.MeasureString(displayName); tw > w-40 {
			displayName = truncate(displayName, 16)
		}
		dc.DrawStringAnchored(displayName, x+w/2, y+h*0.42, 0.5, 0.5)
	}

	// Vote count — colored, below name.
	voteFace, err := face(30, true)
	if err == nil {
		defer voteFace.Close()
		dc.SetFontFace(voteFace)
		dc.SetRGBA255(int(tint.R), int(tint.G), int(tint.B), 255)
		label := "no votes yet"
		if votes > 0 {
			label = formatShortCount(votes) + " " + pluralize(votes, "vote", "votes")
		}
		dc.DrawStringAnchored(label, x+w/2, y+h*0.74, 0.5, 0.5)
	}
}

// drawBracketStats renders the bracket's middle section: the
// description (wrapped up to 2 lines, lighter white) and the current
// round as an accent-colored label below it. Brackets don't have a
// two-contender shape, so this fills the same visual slot as the
// matchup contender tiles.
//
// ItemA carries the bracket description; ItemB carries the "Round N"
// label. Handler composes both.
func drawBracketStats(dc *gg.Context, in ImageInput) error {
	description := strings.TrimSpace(in.ItemA)
	round := strings.TrimSpace(in.ItemB)
	if round == "" {
		round = "Tournament bracket"
	}

	const (
		maxWidth     = float64(imgWidth) - 120
		descTop      = 350.0
		roundY       = 470.0
		descLineGap  = 1.18
		descFontSize = 26.0
	)

	// Description — lightest body text. Skipped entirely when the
	// bracket has none, so the round label moves up naturally.
	if description != "" {
		lines, ok := wrapText(description, descFontSize, maxWidth, 2)
		if !ok || len(lines) == 0 {
			lines = []string{truncate(description, 90)}
		}
		f, err := face(descFontSize, false)
		if err != nil {
			return err
		}
		dc.SetFontFace(f)
		dc.SetRGBA255(0xcb, 0xd5, 0xe1, 255) // muted slate
		for i, line := range lines {
			dc.DrawString(line, 60, descTop+float64(i)*descFontSize*descLineGap)
		}
		f.Close()
	}

	// Round — accent color, bold. Anchored low enough that even a
	// two-line description doesn't collide.
	roundFace, err := face(28, true)
	if err != nil {
		return err
	}
	defer roundFace.Close()
	dc.SetFontFace(roundFace)
	dc.SetRGBA255(0x94, 0xa3, 0xff, 255)
	roundYAdjusted := roundY
	if description == "" {
		// If no description, center the round label vertically in the
		// middle band so the card doesn't feel top-heavy.
		roundYAdjusted = 405
	}
	dc.DrawStringAnchored(round, 60, roundYAdjusted, 0, 0.5)
	return nil
}

// drawAuthorAndEngagement paints the bottom row: colored avatar circle
// carrying the author's initial, the "@username" handle, and a
// middle-dot engagement line. Left-aligned, runs the width of the
// image. Grounded near y=565 with 40px of baseline padding below so
// the accent color stays clearly separated.
func drawAuthorAndEngagement(dc *gg.Context, in ImageInput, accent []color.RGBA) error {
	const (
		rowY     = 570.0
		leftX    = 60.0
		avatarSz = 48.0
	)

	// Avatar circle — hash-derived gradient color fill, bold initial
	// centered. If we have no username, still render an anonymous
	// "M" circle so the row never looks empty.
	initial := "?"
	authorText := "on Matchup"
	if in.AuthorName != "" {
		authorText = "@" + in.AuthorName
		r := []rune(in.AuthorName)
		if len(r) > 0 {
			initial = strings.ToUpper(string(r[0]))
		}
	}

	// Gradient-filled avatar: use the left accent as the avatar tint.
	dc.SetRGBA255(int(accent[0].R), int(accent[0].G), int(accent[0].B), 255)
	dc.DrawCircle(leftX+avatarSz/2, rowY, avatarSz/2)
	dc.Fill()

	// Avatar initial (white, bold).
	initFace, err := face(22, true)
	if err != nil {
		return err
	}
	defer initFace.Close()
	dc.SetFontFace(initFace)
	dc.SetRGBA255(0x0f, 0x17, 0x2a, 255)
	dc.DrawStringAnchored(initial, leftX+avatarSz/2, rowY, 0.5, 0.5)

	// Author handle (bold white).
	handleFace, err := face(22, true)
	if err != nil {
		return err
	}
	defer handleFace.Close()
	dc.SetFontFace(handleFace)
	dc.SetRGBA255(0xf8, 0xfa, 0xfc, 255)
	handleX := leftX + avatarSz + 14
	dc.DrawStringAnchored(authorText, handleX, rowY, 0, 0.5)
	handleW, _ := dc.MeasureString(authorText)

	// Engagement stats, middle-dot separated. Only include pieces that
	// have a non-zero value to keep the line tight.
	var parts []string
	if totalVotes := in.VotesA + in.VotesB; totalVotes > 0 {
		parts = append(parts, formatShortCount(totalVotes)+" "+pluralize(totalVotes, "vote", "votes"))
	}
	if in.Likes > 0 {
		parts = append(parts, formatShortCount(in.Likes)+" "+pluralize(in.Likes, "like", "likes"))
	}
	if in.Comments > 0 {
		parts = append(parts, formatShortCount(in.Comments)+" "+pluralize(in.Comments, "comment", "comments"))
	}
	if len(parts) > 0 {
		statsFace, err := face(20, false)
		if err != nil {
			return err
		}
		defer statsFace.Close()
		dc.SetFontFace(statsFace)
		dc.SetRGBA255(0x94, 0xa3, 0xff, 255)
		stats := " · " + strings.Join(parts, " · ")
		dc.DrawStringAnchored(stats, handleX+handleW, rowY, 0, 0.5)
	}

	return nil
}

// formatShortCount compresses big numbers into a social-style short
// form: 4200 → "4.2K", 3_100_000 → "3.1M". Numbers under 1000 render
// unchanged.
func formatShortCount(n int64) string {
	if n < 0 {
		n = -n
	}
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

// pluralize returns `singular` when n == 1, `plural` otherwise.
func pluralize(n int64, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// drawText writes a single line of text at (x, y) anchored to the
// baseline. Returns an error if face construction fails.
func drawText(dc *gg.Context, text string, x, y, size float64, bold bool, c color.RGBA) error {
	f, err := face(size, bold)
	if err != nil {
		return err
	}
	defer f.Close()
	dc.SetFontFace(f)
	dc.SetColor(c)
	dc.DrawString(text, x, y)
	return nil
}

