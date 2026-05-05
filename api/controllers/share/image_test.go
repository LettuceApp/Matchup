package share

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
)

// TestRender_Matchup_ShapeAndSize — baseline render integrity check.
// Exact pixel comparisons are brittle (a one-line change to layout
// would break them), so we assert on structural invariants only:
// output decodes as PNG, dimensions are 1200×630, and the payload is
// in the expected order-of-magnitude size band. Any future pixel-diff
// golden tests can slot in next to this one.
func TestRender_Matchup_ShapeAndSize(t *testing.T) {
	png, err := Render(ImageInput{
		Kind:       KindMatchup,
		Title:      "Best Rapper of All Time",
		ItemA:      "Kendrick",
		ItemB:      "Drake",
		VotesA:     120,
		VotesB:     88,
		AuthorName: "cordell",
		ShareURL:   "https://matchup.app/m/Kj3mN9xP",
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(png) < 5000 {
		t.Errorf("PNG too small (%d bytes) — likely empty canvas", len(png))
	}
	if len(png) > 2_000_000 {
		t.Errorf("PNG too large (%d bytes) — exceeds iMessage 1MB/platform limits by too much", len(png))
	}
	assertPNG1200x630(t, png)
}

func TestRender_Bracket_ShapeAndSize(t *testing.T) {
	png, err := Render(ImageInput{
		Kind:       KindBracket,
		Title:      "Greatest NBA Players Ever",
		ItemA:      "Size 16 · Round 1",
		AuthorName: "cordell",
		ShareURL:   "https://matchup.app/b/Kj3mN9xP",
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(png) < 5000 {
		t.Errorf("PNG too small (%d bytes) — likely empty canvas", len(png))
	}
	assertPNG1200x630(t, png)
}

func TestRender_EmptyTitleUsesFallback(t *testing.T) {
	// Never panic on an empty or missing title — an image must always
	// come back so the OG unfurl doesn't break even when data is
	// partial.
	if _, err := Render(ImageInput{Kind: KindMatchup, ShareURL: "https://example/m/abc"}); err != nil {
		t.Errorf("expected render to succeed on empty title, got %v", err)
	}
}

func TestRender_LongTitleAutoFits(t *testing.T) {
	// Auto-fit logic should not error on titles longer than the widest
	// wrap-friendly font size. Regression guard for the `wrapText` ok
	// guard at the smallest size.
	long := strings.Repeat("ABC ", 60) // 240 chars of wordy text
	png, err := Render(ImageInput{
		Kind:       KindMatchup,
		Title:      long,
		AuthorName: "cj",
		ShareURL:   "https://matchup.app/m/abcdefgh",
	})
	if err != nil {
		t.Fatalf("Render on long title: %v", err)
	}
	if len(png) == 0 {
		t.Error("empty PNG on long title")
	}
}

// assertPNG1200x630 decodes the bytes via image/png and checks the
// dimensions match the OG spec. Decode errors fail the test.
func assertPNG1200x630(t *testing.T, raw []byte) {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("png.Decode: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != 1200 || b.Dy() != 630 {
		t.Errorf("dimensions = %dx%d, want 1200x630", b.Dx(), b.Dy())
	}
}

func TestGradientFor_Stable(t *testing.T) {
	// Same title should always hash to the same gradient — the whole
	// point of gradientFor is "visual identity" and it breaks
	// permanently the second a new call path returns a different color.
	a := gradientFor("Best Rapper")
	b := gradientFor("Best Rapper")
	if len(a) != len(b) {
		t.Fatalf("unexpected gradient length mismatch: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("gradientFor not stable at index %d: %v vs %v", i, a[i], b[i])
		}
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in  string
		max int
		out string
	}{
		{"short", 20, "short"},
		{"exactly five", 12, "exactly five"},
		{"this is too long for the limit", 10, "this is to…"},
		{"  padded  ", 20, "padded"},
	}
	for _, c := range cases {
		got := truncate(c.in, c.max)
		if got != c.out {
			t.Errorf("truncate(%q, %d) = %q, want %q", c.in, c.max, got, c.out)
		}
	}
}
