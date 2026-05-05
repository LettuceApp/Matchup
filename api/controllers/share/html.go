package share

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"
)

// ContentKind distinguishes matchup previews from bracket previews in
// every piece of the share pipeline (HTML template choice, og:type
// value, JSON-LD schema type, cache key prefix).
type ContentKind string

const (
	KindMatchup ContentKind = "matchup"
	KindBracket ContentKind = "bracket"
)

// Preview carries every piece of data the OG HTML template needs. Kept
// deliberately narrow — the handler builds one of these from the DB row
// and hands it off; the template shouldn't need anything else.
type Preview struct {
	Kind        ContentKind
	ShortID     string    // 8-char base62
	CanonicalID string    // full public_id (for the SPA deep link)
	Title       string    // matchup/bracket title, unescaped
	Description string    // one-line description suitable for og:description
	AuthorName  string    // for "by @username" in description
	UpdatedAt   time.Time // used to version og:image URL (Facebook cache bust)
	SPAPath     string    // human redirect target, e.g. "/users/cordell/matchup/<uuid>"
}

// ogHTMLTemplate is intentionally small — crawlers want meta tags, not a
// full app. `{{.}}` fields are auto-escaped by html/template so
// user-generated title/description are XSS-safe.
var ogHTMLTemplate = template.Must(template.New("og").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>{{.Title}} — Matchup</title>
<meta name="description" content="{{.Description}}">

<meta property="og:type" content="website">
<meta property="og:site_name" content="Matchup">
<meta property="og:title" content="{{.Title}}">
<meta property="og:description" content="{{.Description}}">
<meta property="og:url" content="{{.CanonicalURL}}">
<meta property="og:image" content="{{.ImageURL}}">
<meta property="og:image:width" content="1200">
<meta property="og:image:height" content="630">
<meta property="og:image:alt" content="{{.Title}} — Matchup preview">

<meta name="twitter:card" content="summary_large_image">
<meta name="twitter:title" content="{{.Title}}">
<meta name="twitter:description" content="{{.Description}}">
<meta name="twitter:image" content="{{.ImageURL}}">
<meta name="twitter:image:alt" content="{{.Title}} — Matchup preview">

<link rel="canonical" href="{{.CanonicalURL}}">

<script type="application/ld+json">
{{.JSONLD}}
</script>
</head>
<body>
<p>{{.Title}} — <a href="{{.SPAURL}}">view on Matchup</a>.</p>
</body>
</html>
`))

// RenderOGHTML writes a minimal HTML document with OG + Twitter + JSON-LD
// metadata for the given preview. originBase is the public-facing URL
// origin (e.g. "https://matchup.app") used to build absolute URLs the
// crawler can fetch.
func RenderOGHTML(w http.ResponseWriter, p Preview, originBase string) error {
	originBase = strings.TrimRight(originBase, "/")

	shortPath := "/m/"
	if p.Kind == KindBracket {
		shortPath = "/b/"
	}
	imgPath := "/og/m/"
	if p.Kind == KindBracket {
		imgPath = "/og/b/"
	}

	// Version the og:image URL with updated_at so Facebook's 30-day
	// scraper cache invalidates when the matchup is edited. See
	// https://developers.facebook.com/docs/sharing/webmasters/crawler/
	version := p.UpdatedAt.Unix()
	if version <= 0 {
		version = time.Now().Unix()
	}

	data := struct {
		Title        string
		Description  string
		CanonicalURL string
		ImageURL     string
		SPAURL       string
		JSONLD       template.JS
	}{
		Title:        truncate(p.Title, 90),
		Description:  truncate(p.Description, 200),
		CanonicalURL: originBase + shortPath + p.ShortID,
		ImageURL:     fmt.Sprintf("%s%s%s.png?v=%d", originBase, imgPath, p.ShortID, version),
		SPAURL:       originBase + p.SPAPath,
		JSONLD:       template.JS(jsonLDFor(p, originBase)),
	}

	// Small cache; short enough that edits surface fast, long enough
	// that bursty crawler re-fetches don't re-render the template.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=600, s-maxage=3600")

	var buf bytes.Buffer
	if err := ogHTMLTemplate.Execute(&buf, data); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

// jsonLDFor returns the schema.org JSON-LD blob for the preview. Good
// for LinkedIn rich unfurls and Google Discover surfacing.
func jsonLDFor(p Preview, originBase string) string {
	// Hand-rolled JSON to keep the dep surface small and guarantee the
	// canonical key ordering Google recommends. Values are
	// template-escaped by Go's html/template when embedded, so plain
	// JSON escaping on strings is enough here.
	title := jsonEscape(truncate(p.Title, 90))
	desc := jsonEscape(truncate(p.Description, 200))
	author := jsonEscape(p.AuthorName)
	canonicalURL := originBase + "/m/" + p.ShortID
	if p.Kind == KindBracket {
		canonicalURL = originBase + "/b/" + p.ShortID
	}
	schemaType := "CreativeWork" // matchup == vote object, bracket == tournament; both "CreativeWork" covers it.
	return fmt.Sprintf(`{
  "@context": "https://schema.org",
  "@type": "%s",
  "name": "%s",
  "description": "%s",
  "url": "%s",
  "author": { "@type": "Person", "name": "%s" }
}`, schemaType, title, desc, canonicalURL, author)
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= max {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:max])) + "…"
}

// jsonEscape minimally escapes a string for inclusion in a JSON string
// literal AND inside an HTML `<script type="application/ld+json">` block.
// Handles the characters RFC 8259 forbids unescaped — backslashes, double
// quotes, control chars — PLUS `<` and `>`, which are valid inside a JSON
// literal but can break out of the surrounding <script> tag if the user
// types `</script>` in their title. Escaping `<` as `\u003c` is RFC 8259
// compliant and disables the HTML attack path entirely.
func jsonEscape(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '<':
			b.WriteString(`\u003c`)
		case '>':
			b.WriteString(`\u003e`)
		case '&':
			b.WriteString(`\u0026`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}
