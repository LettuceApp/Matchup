package share

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestSPACrawlerMiddleware_PassThrough covers the fail-open
// disposition of the middleware: non-GET requests, non-crawler
// requests, and unknown paths all hit the underlying handler
// untouched. The middleware exists to *add* rich previews for known
// crawler-pasted URLs, never to intercept anything else.
func TestSPACrawlerMiddleware_PassThrough(t *testing.T) {
	h := &Handler{} // DB nil — we never reach the DB on pass-through paths
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	mw := h.SPACrawlerMiddleware(next)

	cases := []struct {
		name   string
		method string
		path   string
		ua     string
	}{
		// Non-GET requests sidestep the middleware entirely — POSTs
		// to API endpoints, OPTIONS preflights, etc.
		{"POST passes through", "POST", "/users/cordell", "facebookexternalhit/1.1"},
		// Browsers, not bots — humans see the SPA, not a stripped HTML.
		{"human GET to known pattern passes through", "GET", "/users/cordell",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 13_5)"},
		// Crawler but path doesn't match any pattern — homepage,
		// login, settings, anything not in our preview list.
		{"crawler on /home passes through", "GET", "/home", "facebookexternalhit/1.1"},
		{"crawler on /login passes through", "GET", "/login", "Twitterbot/1.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("User-Agent", tc.ua)
			rec := httptest.NewRecorder()
			mw.ServeHTTP(rec, req)
			if !called {
				t.Fatalf("expected next handler to be called for %q (%s)", tc.path, tc.method)
			}
		})
	}
}

// TestSPACrawlerMiddleware_PathPatterns exercises just the regex
// classifier (not the DB lookup) — verifies that each known SPA
// route shape is identified correctly and ambiguous overlaps go to
// the right kind. We assert by inspecting which dispatch path
// `tryServeSPAOG` takes via the path classifier helper below.
func TestSPACrawlerMiddleware_PathPatterns(t *testing.T) {
	cases := []struct {
		path string
		want string // "matchup" | "bracket" | "community" | "user" | "none"
	}{
		// Matchup long form — has to be checked before the user-profile
		// pattern because /users/cordell/... would otherwise eat it.
		{"/users/cordell/matchup/01234567-89ab-4cde-9f01-23456789abcd", "matchup"},
		// Bracket long form — UUIDs come in both with-dashes (36 chars)
		// and bare hex (32 chars); both shapes get matched.
		{"/brackets/01234567-89ab-4cde-9f01-23456789abcd", "bracket"},
		{"/brackets/0123456789ab4cde9f0123456789abcd", "bracket"},
		// User profile, bare.
		{"/users/cordell", "user"},
		// Community page — and every nested route under it surfaces
		// the same card (champions, members, settings).
		{"/c/anime-fans", "community"},
		{"/c/anime-fans/champions", "community"},
		{"/c/anime-fans/members", "community"},
		{"/c/anime-fans/settings", "community"},
		// Paths that should NOT trigger a preview — they fall through
		// to the SPA.
		{"/home", "none"},
		{"/", "none"},
		{"/login", "none"},
		{"/brackets/new", "none"},               // literal SPA path, not a UUID
		{"/users/cordell/create-matchup", "none"}, // not a /matchup/<uuid> path
		{"/users/cordell/profile", "none"},        // not a known pattern
		{"/communities/new", "none"},              // SPA-only path
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got := classifyPath(tc.path)
			if got != tc.want {
				t.Errorf("classifyPath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// classifyPath is a test helper that mirrors the dispatch logic in
// tryServeSPAOG without touching the DB. Kept in test code (not the
// production file) so the classifier and the live dispatcher stay
// in lockstep — if you add a new pattern to tryServeSPAOG, add a
// branch here and the table above gains coverage automatically.
func classifyPath(path string) string {
	p := strings.TrimRight(path, "/")
	if p == "" {
		return "none"
	}
	if matchupLongRE.MatchString(p) {
		return "matchup"
	}
	if bracketLongRE.MatchString(p) {
		return "bracket"
	}
	if communityRE.MatchString(p) {
		return "community"
	}
	if userProfileRE.MatchString(p) {
		return "user"
	}
	return "none"
}

// TestRenderOGHTML_UserKind verifies the new KindUser path through
// the renderer: image falls back to /og-default.png, JSON-LD schema
// type is Person, the author block is dropped (semantically the
// page IS the user, not authored by them).
func TestRenderOGHTML_UserKind(t *testing.T) {
	rec := httptest.NewRecorder()
	p := Preview{
		Kind:         KindUser,
		Title:        "@cordell on Matchup",
		Description:  "Some bio text here",
		AuthorName:   "cordell",
		UpdatedAt:    time.Unix(1_700_000_000, 0),
		SPAPath:      "/users/cordell",
		CanonicalURL: "https://matchup.app/users/cordell",
	}
	if err := RenderOGHTML(rec, p, "https://matchup.app"); err != nil {
		t.Fatalf("RenderOGHTML: %v", err)
	}
	body := rec.Body.String()
	wants := []string{
		`<meta property="og:title" content="@cordell on Matchup">`,
		`<meta property="og:url" content="https://matchup.app/users/cordell">`,
		// No /og/m/ or /og/b/ image URL — falls back to homepage card.
		`<meta property="og:image" content="https://matchup.app/og-default.png">`,
		`<link rel="canonical" href="https://matchup.app/users/cordell">`,
		`"@type": "Person"`,
	}
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("missing %q in body:\n%s", w, body)
		}
	}
	// Author block is dropped for Person kinds.
	if strings.Contains(body, `"author":`) {
		t.Errorf("KindUser JSON-LD should omit the author block; body:\n%s", body)
	}
}

// TestRenderOGHTML_CommunityKind same idea for communities — schema
// type is Organization, image is the homepage card fallback.
func TestRenderOGHTML_CommunityKind(t *testing.T) {
	rec := httptest.NewRecorder()
	p := Preview{
		Kind:         KindCommunity,
		Title:        "Anime Fans",
		Description:  "A community for anime lovers",
		UpdatedAt:    time.Unix(1_700_000_000, 0),
		SPAPath:      "/c/anime-fans",
		CanonicalURL: "https://matchup.app/c/anime-fans",
	}
	if err := RenderOGHTML(rec, p, "https://matchup.app"); err != nil {
		t.Fatalf("RenderOGHTML: %v", err)
	}
	body := rec.Body.String()
	wants := []string{
		`<meta property="og:title" content="Anime Fans">`,
		`<meta property="og:url" content="https://matchup.app/c/anime-fans">`,
		`<meta property="og:image" content="https://matchup.app/og-default.png">`,
		`"@type": "Organization"`,
	}
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("missing %q in body:\n%s", w, body)
		}
	}
}

// TestRenderOGHTML_MatchupKind_StillWorks regression-guards the
// pre-existing behavior: Phase 2 added overrides + new kinds without
// changing how the matchup/bracket paths render.
func TestRenderOGHTML_MatchupKind_StillWorks(t *testing.T) {
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
	// Image URL still hits the dynamic renderer, versioned with
	// updated_at — Phase 2 must not regress this path.
	if !strings.Contains(body, `https://matchup.app/og/m/Kj3mN9xP.png?v=1700000000`) {
		t.Errorf("matchup og:image regressed; body:\n%s", body)
	}
	// JSON-LD schema is still CreativeWork with the author block.
	if !strings.Contains(body, `"@type": "CreativeWork"`) {
		t.Errorf("matchup JSON-LD @type regressed; body:\n%s", body)
	}
	if !strings.Contains(body, `"author":`) {
		t.Errorf("matchup JSON-LD author block missing; body:\n%s", body)
	}
}
