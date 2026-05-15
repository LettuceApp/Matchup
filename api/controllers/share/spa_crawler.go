package share

// spa_crawler.go intercepts SPA-route GETs from known link-preview
// crawlers and serves rich OG HTML inline, so a long URL pasted into
// iMessage / Slack / Discord / X unfurls with the actual matchup
// (or bracket / user / community) title, description, and image
// instead of the generic homepage card.
//
// Why a middleware, not chi route handlers? Chi routes are
// path-keyed: if we registered `/users/{username}/matchup/{id}` at
// the chi layer, a *human* hitting that URL would also be routed to
// our handler, breaking the SPA. The middleware lets us short-circuit
// only the crawler case (User-Agent match) and pass humans through
// to whatever serves the SPA — the local-dev reverse proxy, or the
// CDN/ingress in prod. SPA traffic stays untouched in the hot path.
//
// Patterns intercepted (matched in order — first match wins):
//
//	GET /users/{username}/matchup/{publicID}   → KindMatchup OG, public_id lookup
//	GET /brackets/{publicID}                   → KindBracket OG, public_id lookup
//	GET /users/{username}                      → KindUser OG (no per-user image yet)
//	GET /c/{slug}                              → KindCommunity OG (no per-community image yet)
//	GET /c/{slug}/<anything>                   → KindCommunity OG (champions, members, etc. all share one card)
//
// Any path that doesn't match falls through. POST/PUT/etc. fall
// through. Non-crawler requests fall through. We're deliberately
// fail-open: a lookup miss or an internal error passes the request
// down the chain rather than serving a half-broken card.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"Matchup/models"

	"github.com/jmoiron/sqlx"
)

// Compile path patterns once at init. Public_id is a UUIDv4 in this
// codebase (32 hex chars + 4 dashes); username + slug we match with a
// permissive char class because the canonical regexes for those live
// in the user/community validators and we just need a coarse filter
// here. If a pattern over-matches, the DB lookup will simply miss
// and we fall through.
var (
	matchupLongRE   = regexp.MustCompile(`^/users/([^/]+)/matchup/([0-9a-fA-F-]{32,36})$`)
	bracketLongRE   = regexp.MustCompile(`^/brackets/([0-9a-fA-F-]{32,36})$`)
	userProfileRE   = regexp.MustCompile(`^/users/([^/]+)$`)
	communityRE     = regexp.MustCompile(`^/c/([^/]+)(?:/.*)?$`)
)

// SPACrawlerMiddleware returns chi-compatible middleware. Install at
// the top of the router stack (before share.Mount + connect routes
// + the FRONTEND_PROXY fallback) so it gets first crack at every
// request and only consumes the ones it explicitly handles.
//
// Behavior:
//   - non-GET → pass through.
//   - non-crawler UA → pass through.
//   - GET + crawler + matches a known SPA pattern + lookup succeeds → render OG HTML, return.
//   - anything else → pass through.
func (h *Handler) SPACrawlerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !IsCrawler(r) {
			next.ServeHTTP(w, r)
			return
		}
		if h.tryServeSPAOG(w, r) {
			return
		}
		next.ServeHTTP(w, r)
	})
}

// tryServeSPAOG matches the request path against the known SPA
// preview patterns and serves rich OG HTML if anything hits. Returns
// true when the response was written, false to let the caller pass
// through to the next handler.
func (h *Handler) tryServeSPAOG(w http.ResponseWriter, r *http.Request) bool {
	// Strip trailing slash + query before regex match — crawlers
	// sometimes append both. Done with TrimRight rather than path.Clean
	// so leading slashes stay intact.
	p := strings.TrimRight(r.URL.Path, "/")
	if p == "" {
		return false
	}
	origin := h.originFromRequest(r)

	// /users/{username}/matchup/{publicID} — most specific pattern,
	// check first so the bare-profile regex below doesn't shadow it.
	if m := matchupLongRE.FindStringSubmatch(p); m != nil {
		return h.serveMatchupOG(w, r, m[2], origin)
	}

	// /brackets/{publicID}
	if m := bracketLongRE.FindStringSubmatch(p); m != nil {
		return h.serveBracketOG(w, r, m[1], origin)
	}

	// /c/{slug} (and any subpath under it — /champions, /members,
	// /settings all surface the same community card; finer-grained
	// per-tab previews are out of scope for v1).
	if m := communityRE.FindStringSubmatch(p); m != nil {
		return h.serveCommunityOG(w, r, m[1], origin)
	}

	// /users/{username} (must be checked AFTER the /users/{u}/matchup/...
	// case above, since this pattern would otherwise match the longer
	// path with username = "u" or similar).
	if m := userProfileRE.FindStringSubmatch(p); m != nil {
		return h.serveUserOG(w, r, m[1], origin)
	}

	return false
}

// ---- per-kind handlers ----------------------------------------------

func (h *Handler) serveMatchupOG(w http.ResponseWriter, r *http.Request, publicID, origin string) bool {
	m, err := findMatchupByPublicID(r.Context(), h.readPool(), publicID)
	if err != nil {
		// On lookup miss, fall through — humans get the SPA's own
		// 404 page, crawlers get the homepage card via the index.html
		// fallback. Either way better than serving a hollow OG.
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("spa-crawler: matchup lookup %q: %v", publicID, err)
		}
		return false
	}
	preview := matchupPreview(m, origin)
	if err := RenderOGHTML(w, preview, origin); err != nil {
		log.Printf("spa-crawler: render matchup HTML %q: %v", publicID, err)
		return false
	}
	return true
}

func (h *Handler) serveBracketOG(w http.ResponseWriter, r *http.Request, publicID, origin string) bool {
	b, err := findBracketByPublicID(r.Context(), h.readPool(), publicID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("spa-crawler: bracket lookup %q: %v", publicID, err)
		}
		return false
	}
	preview := bracketPreview(b, origin)
	if err := RenderOGHTML(w, preview, origin); err != nil {
		log.Printf("spa-crawler: render bracket HTML %q: %v", publicID, err)
		return false
	}
	return true
}

func (h *Handler) serveUserOG(w http.ResponseWriter, r *http.Request, username, origin string) bool {
	u, err := findUserByUsername(r.Context(), h.readPool(), username)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("spa-crawler: user lookup %q: %v", username, err)
		}
		return false
	}
	// Private accounts surface a generic "private profile on Matchup"
	// card — same posture as private matchups. Crawlers see no PII.
	if u.IsPrivate {
		preview := Preview{
			Kind:        KindUser,
			Title:       "Private profile on Matchup",
			Description: "This profile is private. Sign in to follow.",
			SPAPath:     "/users/" + url.PathEscape(u.Username),
		}
		if err := RenderOGHTML(w, preview, origin); err != nil {
			log.Printf("spa-crawler: render private-user HTML %q: %v", username, err)
			return false
		}
		return true
	}
	preview := userPreview(u, origin)
	if err := RenderOGHTML(w, preview, origin); err != nil {
		log.Printf("spa-crawler: render user HTML %q: %v", username, err)
		return false
	}
	return true
}

func (h *Handler) serveCommunityOG(w http.ResponseWriter, r *http.Request, slug, origin string) bool {
	c, err := findCommunityBySlug(r.Context(), h.readPool(), slug)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("spa-crawler: community lookup %q: %v", slug, err)
		}
		return false
	}
	preview := communityPreview(c, origin)
	if err := RenderOGHTML(w, preview, origin); err != nil {
		log.Printf("spa-crawler: render community HTML %q: %v", slug, err)
		return false
	}
	return true
}

// ---- lookups --------------------------------------------------------

func findMatchupByPublicID(ctx context.Context, db *sqlx.DB, publicID string) (*models.Matchup, error) {
	var m models.Matchup
	err := sqlx.GetContext(ctx, db, &m,
		"SELECT * FROM matchups WHERE public_id = $1 LIMIT 1", publicID)
	if err != nil {
		return nil, err
	}
	// Author + items for the description / image-URL builders.
	_ = sqlx.GetContext(ctx, db, &m.Author,
		"SELECT * FROM users WHERE id = $1", m.AuthorID)
	return &m, nil
}

func findBracketByPublicID(ctx context.Context, db *sqlx.DB, publicID string) (*models.Bracket, error) {
	var b models.Bracket
	err := sqlx.GetContext(ctx, db, &b,
		"SELECT * FROM brackets WHERE public_id = $1 LIMIT 1", publicID)
	if err != nil {
		return nil, err
	}
	_ = sqlx.GetContext(ctx, db, &b.Author,
		"SELECT * FROM users WHERE id = $1", b.AuthorID)
	return &b, nil
}

func findUserByUsername(ctx context.Context, db *sqlx.DB, username string) (*models.User, error) {
	var u models.User
	// Case-insensitive — usernames are stored lowercased on create but
	// some old rows are mixed case; match both shapes.
	err := sqlx.GetContext(ctx, db, &u,
		`SELECT * FROM users
		 WHERE LOWER(username) = LOWER($1)
		   AND deleted_at IS NULL
		   AND banned_at IS NULL
		 LIMIT 1`, username)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func findCommunityBySlug(ctx context.Context, db *sqlx.DB, slug string) (*models.Community, error) {
	var c models.Community
	err := sqlx.GetContext(ctx, db, &c,
		`SELECT * FROM communities
		 WHERE LOWER(slug) = LOWER($1)
		 LIMIT 1`, slug)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ---- preview builders for new kinds ---------------------------------

func userPreview(u *models.User, originBase string) Preview {
	// Build a friendly description. Bio if present, fallback to a
	// generic "Profile on Matchup" line. Truncation happens in
	// RenderOGHTML so we don't double-truncate here.
	desc := strings.TrimSpace(u.Bio)
	if desc == "" {
		desc = fmt.Sprintf("@%s's profile on Matchup — vote, bracket, decide.", u.Username)
	}
	spaPath := "/users/" + url.PathEscape(u.Username)
	return Preview{
		Kind:         KindUser,
		Title:        "@" + u.Username + " on Matchup",
		Description:  desc,
		AuthorName:   u.Username,
		UpdatedAt:    u.UpdatedAt,
		SPAPath:      spaPath,
		CanonicalURL: originBase + spaPath,
		// ImageURL left empty → renderer falls back to /og-default.png.
		// Per-user OG image renderer is Phase 3 work.
	}
}

func communityPreview(c *models.Community, originBase string) Preview {
	desc := strings.TrimSpace(c.Description)
	if desc == "" {
		desc = fmt.Sprintf("/c/%s — a community on Matchup.", c.Slug)
	}
	spaPath := "/c/" + url.PathEscape(c.Slug)
	return Preview{
		Kind:         KindCommunity,
		Title:        c.Name,
		Description:  desc,
		UpdatedAt:    c.UpdatedAt,
		SPAPath:      spaPath,
		CanonicalURL: originBase + spaPath,
		// ImageURL left empty → /og-default.png. Phase 3 will swap in
		// a per-community card with the picked theme gradient.
	}
}
