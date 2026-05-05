package share

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRenderOGHTML_Matchup_HasAllExpectedMetaTags(t *testing.T) {
	// Every meta tag the social crawlers actually read must be present
	// in the output. Adding a bot is cheap; forgetting to render a tag
	// is an invisible regression until someone shares a link in Slack
	// and it unfurls as a plain URL.
	rec := httptest.NewRecorder()
	p := Preview{
		Kind:        KindMatchup,
		ShortID:     "Kj3mN9xP",
		CanonicalID: "some-uuid",
		Title:       "Best Rapper of All Time",
		Description: "Vote on this matchup by @cordell on Matchup.",
		AuthorName:  "cordell",
		UpdatedAt:   time.Unix(1_700_000_000, 0),
		SPAPath:     "/users/cordell/matchup/some-uuid",
	}
	if err := RenderOGHTML(rec, p, "https://matchup.app"); err != nil {
		t.Fatalf("RenderOGHTML: %v", err)
	}

	body := rec.Body.String()
	required := []string{
		`<meta property="og:type" content="website">`,
		`<meta property="og:site_name" content="Matchup">`,
		`<meta property="og:title" content="Best Rapper of All Time">`,
		`<meta property="og:url" content="https://matchup.app/m/Kj3mN9xP">`,
		`<meta property="og:image:width" content="1200">`,
		`<meta property="og:image:height" content="630">`,
		`<meta name="twitter:card" content="summary_large_image">`,
		`<link rel="canonical" href="https://matchup.app/m/Kj3mN9xP">`,
		`application/ld+json`,
	}
	for _, needle := range required {
		if !strings.Contains(body, needle) {
			t.Errorf("missing tag: %q\nbody:\n%s", needle, body)
		}
	}

	// og:image must be versioned with updated_at Unix so Facebook's
	// 30d scraper cache can invalidate on edit.
	if !strings.Contains(body, "?v=1700000000") {
		t.Errorf("og:image missing versioned ?v= query (cache bust); body:\n%s", body)
	}
}

func TestRenderOGHTML_Bracket_UsesBracketShortPath(t *testing.T) {
	rec := httptest.NewRecorder()
	p := Preview{
		Kind:        KindBracket,
		ShortID:     "Xy1aBCDE",
		CanonicalID: "bracket-uuid",
		Title:       "Greatest Rapper Tournament",
		Description: "Tournament bracket by @cordell on Matchup.",
		AuthorName:  "cordell",
		UpdatedAt:   time.Now(),
		SPAPath:     "/brackets/bracket-uuid",
	}
	if err := RenderOGHTML(rec, p, "https://matchup.app"); err != nil {
		t.Fatalf("RenderOGHTML: %v", err)
	}
	body := rec.Body.String()

	if !strings.Contains(body, `"https://matchup.app/b/Xy1aBCDE"`) {
		t.Errorf("bracket short URL /b/ not present; body:\n%s", body)
	}
	if !strings.Contains(body, `https://matchup.app/og/b/Xy1aBCDE.png`) {
		t.Errorf("bracket og:image path /og/b/ not present; body:\n%s", body)
	}
}

func TestRenderOGHTML_EscapesUserInputTitle(t *testing.T) {
	// html/template auto-escapes via text interpolation — this test
	// locks that in so nobody accidentally switches to fmt.Sprintf and
	// invites XSS through a user-supplied matchup title.
	rec := httptest.NewRecorder()
	p := Preview{
		Kind:      KindMatchup,
		ShortID:   "aaaaaaaa",
		Title:     `<script>alert("xss")</script>`,
		UpdatedAt: time.Now(),
		SPAPath:   "/",
	}
	if err := RenderOGHTML(rec, p, "https://matchup.app"); err != nil {
		t.Fatalf("RenderOGHTML: %v", err)
	}
	body := rec.Body.String()
	if strings.Contains(body, "<script>alert") {
		t.Error("raw script tag made it into HTML output — XSS risk")
	}
	// The escaped form must be present for crawlers to display
	// something useful.
	if !strings.Contains(body, "&lt;script&gt;") && !strings.Contains(body, "&#34;") {
		t.Errorf("expected escaped title in body, got:\n%s", body)
	}
}

func TestJSONEscape(t *testing.T) {
	// Minimal sanity: the characters forbidden unescaped by RFC 8259
	// all get a backslash-escape form.
	got := jsonEscape(`quote:" backslash:\ nl:
tab:	`)
	for _, want := range []string{`\"`, `\\`, `\n`, `\t`} {
		if !strings.Contains(got, want) {
			t.Errorf("jsonEscape missing %q in output: %q", want, got)
		}
	}
}

func TestTruncateUnicode(t *testing.T) {
	// truncate counts RUNES, not bytes, so multi-byte characters
	// shouldn't bloat past the max.
	// 10 double-byte characters → should all fit at max=10.
	s := "日本語日本語日本語日"
	got := truncate(s, 10)
	if got != s {
		t.Errorf("rune-count truncate mishandled Unicode; got %q", got)
	}

	// 11 double-byte characters → last gets chopped and "…" appended.
	s = "日本語日本語日本語日A"
	got = truncate(s, 10)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipsis on over-length; got %q", got)
	}
}
