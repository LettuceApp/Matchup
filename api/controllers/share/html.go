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
//
// `KindUser` and `KindCommunity` were added in Phase 2 of the share
// work — the crawler middleware uses them to serve rich previews for
// long SPA URLs that don't go through the /m/{short} or /b/{short}
// gateway. They don't have their own rendered preview images yet
// (Phase 3 work); the HTML renderer falls back to the homepage card
// when ImageURL is unset for these kinds.
type ContentKind string

const (
	KindMatchup   ContentKind = "matchup"
	KindBracket   ContentKind = "bracket"
	KindUser      ContentKind = "user"
	KindCommunity ContentKind = "community"
)

// Preview carries every piece of data the OG HTML template needs. Kept
// deliberately narrow — the handler builds one of these from the DB row
// and hands it off; the template shouldn't need anything else.
//
// `ImageURL` / `CanonicalURL` are optional overrides for kinds that
// don't have a /m/{short_id} or /b/{short_id} canonical short URL —
// user profiles and communities, which we surface via the long SPA
// URL directly. When unset, the renderer derives them from
// Kind + ShortID like before (matchup → /m/{short}, bracket → /b/{short}).
type Preview struct {
	Kind         ContentKind
	ShortID      string    // 8-char base62 (matchup/bracket only)
	CanonicalID  string    // full public_id (for the SPA deep link)
	Title        string    // matchup/bracket title, unescaped
	Description  string    // one-line description suitable for og:description
	AuthorName   string    // for "by @username" in description
	UpdatedAt    time.Time // used to version og:image URL (Facebook cache bust)
	SPAPath      string    // human redirect target, e.g. "/users/cordell/matchup/<uuid>"
	ImageURL     string    // optional override; empty means derive from Kind+ShortID
	CanonicalURL string    // optional override for the og:url + JSON-LD url
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
//
// For matchup/bracket previews, the canonical URL + image URL default
// to the short-URL forms (/m/{short_id}, /og/m/{short_id}.png). For
// user/community previews and any caller that needs to override, the
// Preview's CanonicalURL + ImageURL fields take precedence — useful
// when the SPA route itself is the canonical surface (no short URL)
// or when a per-kind rendered image isn't available yet and we want
// to fall back to the homepage card.
func RenderOGHTML(w http.ResponseWriter, p Preview, originBase string) error {
	originBase = strings.TrimRight(originBase, "/")

	// Version the og:image URL with updated_at so Facebook's 30-day
	// scraper cache invalidates when the matchup is edited. See
	// https://developers.facebook.com/docs/sharing/webmasters/crawler/
	version := p.UpdatedAt.Unix()
	if version <= 0 {
		version = time.Now().Unix()
	}

	// Canonical URL — explicit override wins; otherwise derive from
	// Kind + ShortID for matchups/brackets, or fall back to the SPA
	// path as the canonical surface for user/community kinds.
	canonicalURL := p.CanonicalURL
	if canonicalURL == "" {
		switch p.Kind {
		case KindBracket:
			canonicalURL = originBase + "/b/" + p.ShortID
		case KindMatchup:
			canonicalURL = originBase + "/m/" + p.ShortID
		default:
			canonicalURL = originBase + p.SPAPath
		}
	}

	// Image URL — explicit override wins. Matchup/bracket defaults
	// hit the dynamic /og/{m|b}/{short}.png renderer (versioned with
	// updated_at for Facebook's 30-day cache). User/community kinds
	// fall back to the homepage social card (/og-default.png) until
	// per-kind image renderers ship in Phase 3.
	imageURL := p.ImageURL
	if imageURL == "" {
		switch p.Kind {
		case KindBracket:
			imageURL = fmt.Sprintf("%s/og/b/%s.png?v=%d", originBase, p.ShortID, version)
		case KindMatchup:
			imageURL = fmt.Sprintf("%s/og/m/%s.png?v=%d", originBase, p.ShortID, version)
		default:
			imageURL = originBase + "/og-default.png"
		}
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
		CanonicalURL: canonicalURL,
		ImageURL:     imageURL,
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

	// Honor the override when set (user/community kinds) — otherwise
	// derive the canonical URL from Kind + ShortID like before.
	canonicalURL := p.CanonicalURL
	if canonicalURL == "" {
		switch p.Kind {
		case KindBracket:
			canonicalURL = originBase + "/b/" + p.ShortID
		case KindMatchup:
			canonicalURL = originBase + "/m/" + p.ShortID
		default:
			canonicalURL = originBase + p.SPAPath
		}
	}

	// Map our domain kinds onto schema.org types. CreativeWork covers
	// matchups (a vote object) + brackets (a tournament) since both
	// fit the "user-generated work" mold. Users surface as Person.
	// Communities as Organization.
	schemaType := "CreativeWork"
	switch p.Kind {
	case KindUser:
		schemaType = "Person"
	case KindCommunity:
		schemaType = "Organization"
	}

	// For Person/Organization kinds, "author" doesn't make semantic
	// sense in schema.org's vocabulary — the page IS the person/org.
	// Drop the author block to avoid Google flagging it as malformed.
	if p.Kind == KindUser || p.Kind == KindCommunity {
		return fmt.Sprintf(`{
  "@context": "https://schema.org",
  "@type": "%s",
  "name": "%s",
  "description": "%s",
  "url": "%s"
}`, schemaType, title, desc, canonicalURL)
	}

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
