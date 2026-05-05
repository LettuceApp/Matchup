package share

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"Matchup/models"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// NOTE: time is imported transitively via models.Matchup.UpdatedAt usage.

// Handler carries everything the share routes need. Construct one via
// NewHandler and mount its routes on a chi router. Keeps package state
// injectable so tests don't touch globals.
type Handler struct {
	DB             *sqlx.DB
	ReadDB         *sqlx.DB // optional replica
	OriginBase     string   // e.g. "https://matchup.app" — used for absolute URLs in OG tags
	FrontendOrigin string   // absolute host where the SPA lives (e.g. a separate tunnel in local dev)
	ImageLimiter   *RateLimiter
}

// NewHandler builds a Handler with sensible defaults.
//
//	PUBLIC_ORIGIN   — absolute URL bots should see in OG `og:url`/`og:image`.
//	                  Falls back to request scheme+host when unset.
//	FRONTEND_ORIGIN — absolute URL the SPA is served from. When set, the
//	                  human-redirect on /m/{id} and /b/{id} goes to
//	                  FRONTEND_ORIGIN + SPA path. Needed locally because
//	                  the API tunnel and frontend tunnel are different
//	                  hostnames. In production it's the same hostname as
//	                  PUBLIC_ORIGIN (or empty to default to it).
func NewHandler(db, readDB *sqlx.DB) *Handler {
	base := strings.TrimRight(os.Getenv("PUBLIC_ORIGIN"), "/")
	feBase := strings.TrimRight(os.Getenv("FRONTEND_ORIGIN"), "/")
	return &Handler{
		DB:             db,
		ReadDB:         readDB,
		OriginBase:     base,
		FrontendOrigin: feBase,
		// 60 rps global / burst 20, 10 rps per-short_id / burst 5.
		ImageLimiter: NewRateLimiter(60, 20, 10, 5),
	}
}

// redirectTargetForHuman joins FRONTEND_ORIGIN (if set) with the SPA
// path so the browser lands on the React app rather than the API. When
// FRONTEND_ORIGIN is unset, returns the relative path — callers on the
// same origin as the SPA get the right behavior by default.
func (h *Handler) redirectTargetForHuman(spaPath, rawQuery string) string {
	target := spaPath
	if h.FrontendOrigin != "" {
		target = h.FrontendOrigin + spaPath
	}
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	return target
}

// Mount registers all share-related routes on r. Call this BEFORE the
// Connect RPC router is mounted, so short-URL patterns don't collide.
func (h *Handler) Mount(r chi.Router) {
	r.Get("/m/{shortID}", h.shareMatchup)
	r.Get("/b/{shortID}", h.shareBracket)
	r.Get("/og/m/{shortID}.png", h.ogMatchupImage)
	r.Get("/og/b/{shortID}.png", h.ogBracketImage)
}

// originFromRequest determines the canonical origin to use for
// absolute URLs in OG metadata. Priority: configured PUBLIC_ORIGIN env,
// then request's own scheme+host, then "http://localhost".
func (h *Handler) originFromRequest(r *http.Request) string {
	if h.OriginBase != "" {
		return h.OriginBase
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	if host == "" {
		host = "localhost"
	}
	return scheme + "://" + host
}

// ---- matchup -------------------------------------------------------

func (h *Handler) shareMatchup(w http.ResponseWriter, r *http.Request) {
	shortID := chi.URLParam(r, "shortID")
	if !validShortID(shortID) {
		http.NotFound(w, r)
		return
	}

	m, err := findMatchupByShortID(r.Context(), h.readPool(), shortID)
	if err != nil {
		respondNotFoundOr410(w, r, err)
		return
	}

	if !IsCrawler(r) {
		// Humans get redirected to the full SPA route, with the
		// incoming ref preserved so attribution works. When
		// FRONTEND_ORIGIN is set, the redirect is absolute so humans
		// land on the separately-hosted SPA rather than 404ing on the
		// API tunnel.
		http.Redirect(w, r, h.redirectTargetForHuman(matchupSPAPath(m), r.URL.RawQuery), http.StatusFound)
		return
	}

	preview := matchupPreview(m, h.originFromRequest(r))
	if err := RenderOGHTML(w, preview, h.originFromRequest(r)); err != nil {
		log.Printf("share: render matchup html %s: %v", shortID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func (h *Handler) ogMatchupImage(w http.ResponseWriter, r *http.Request) {
	shortID := chi.URLParam(r, "shortID")
	shortID = strings.TrimSuffix(shortID, ".png")
	if !validShortID(shortID) {
		http.NotFound(w, r)
		return
	}
	if ok, reason := h.ImageLimiter.Allow(shortID); !ok {
		log.Printf("share: ratelimit %s (bucket=%s)", shortID, reason)
		w.Header().Set("Retry-After", "1")
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}

	m, err := findMatchupByShortID(r.Context(), h.readPool(), shortID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_ = SetMiss(r.Context(), KindMatchup, shortID)
			h.writeFallbackPNG(w, r, shortID, KindMatchup)
			return
		}
		log.Printf("share: lookup matchup %s: %v", shortID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Private matchups render a generic card, not the title.
	if shouldObscure(m) {
		h.writeObscuredPNG(w, r, KindMatchup)
		return
	}

	updatedAt := m.UpdatedAt.Unix()
	if cached, err := Get(r.Context(), KindMatchup, shortID, updatedAt); err == nil {
		writePNG(w, cached, shortID, updatedAt)
		return
	} else if errors.Is(err, ErrNegativeCacheHit) {
		h.writeFallbackPNG(w, r, shortID, KindMatchup)
		return
	} else if !errors.Is(err, ErrCacheMiss) {
		log.Printf("share: cache get %s: %v", shortID, err)
	}

	// Load items (the matchup struct scanned via SELECT * doesn't
	// include them — they live in a join table).
	items, err := loadMatchupItems(r.Context(), h.readPool(), m.ID)
	if err != nil {
		log.Printf("share: load items %s: %v", shortID, err)
		items = nil
	}

	in := ImageInput{
		Kind:       KindMatchup,
		Title:      m.Title,
		AuthorName: authorName(m.Author.Username),
		Likes:      int64(m.LikesCount),
		Comments:   int64(m.CommentsCount),
		ShareURL:   h.originFromRequest(r) + "/m/" + shortID,
	}
	if len(items) > 0 {
		in.ItemA = items[0].Item
		in.VotesA = int64(items[0].Votes)
	}
	if len(items) > 1 {
		in.ItemB = items[1].Item
		in.VotesB = int64(items[1].Votes)
	}

	png, err := Render(in)
	if err != nil {
		log.Printf("share: render matchup png %s: %v", shortID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	_ = Set(r.Context(), KindMatchup, shortID, updatedAt, png)
	writePNG(w, png, shortID, updatedAt)
}

// ---- bracket -------------------------------------------------------

func (h *Handler) shareBracket(w http.ResponseWriter, r *http.Request) {
	shortID := chi.URLParam(r, "shortID")
	if !validShortID(shortID) {
		http.NotFound(w, r)
		return
	}

	b, err := findBracketByShortID(r.Context(), h.readPool(), shortID)
	if err != nil {
		respondNotFoundOr410(w, r, err)
		return
	}

	if !IsCrawler(r) {
		http.Redirect(w, r, h.redirectTargetForHuman(bracketSPAPath(b), r.URL.RawQuery), http.StatusFound)
		return
	}

	preview := bracketPreview(b, h.originFromRequest(r))
	if err := RenderOGHTML(w, preview, h.originFromRequest(r)); err != nil {
		log.Printf("share: render bracket html %s: %v", shortID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func (h *Handler) ogBracketImage(w http.ResponseWriter, r *http.Request) {
	shortID := chi.URLParam(r, "shortID")
	shortID = strings.TrimSuffix(shortID, ".png")
	if !validShortID(shortID) {
		http.NotFound(w, r)
		return
	}
	if ok, reason := h.ImageLimiter.Allow(shortID); !ok {
		log.Printf("share: ratelimit %s (bucket=%s)", shortID, reason)
		w.Header().Set("Retry-After", "1")
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}

	b, err := findBracketByShortID(r.Context(), h.readPool(), shortID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_ = SetMiss(r.Context(), KindBracket, shortID)
			h.writeFallbackPNG(w, r, shortID, KindBracket)
			return
		}
		log.Printf("share: lookup bracket %s: %v", shortID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if bracketShouldObscure(b) {
		h.writeObscuredPNG(w, r, KindBracket)
		return
	}

	updatedAt := b.UpdatedAt.Unix()
	if cached, err := Get(r.Context(), KindBracket, shortID, updatedAt); err == nil {
		writePNG(w, cached, shortID, updatedAt)
		return
	} else if errors.Is(err, ErrNegativeCacheHit) {
		h.writeFallbackPNG(w, r, shortID, KindBracket)
		return
	} else if !errors.Is(err, ErrCacheMiss) {
		log.Printf("share: cache get %s: %v", shortID, err)
	}

	in := ImageInput{
		Kind:       KindBracket,
		Title:      b.Title,
		AuthorName: authorName(b.Author.Username),
		Likes:      int64(b.LikesCount),
		Comments:   int64(b.CommentsCount),
		ShareURL:   h.originFromRequest(r) + "/b/" + shortID,
		// For brackets, the renderer piggy-backs on ItemA/ItemB to
		// pass a description subtitle + a round label. See
		// image.go::drawBracketStats.
		ItemA: b.Description,
		ItemB: fmt.Sprintf("Round %d", b.CurrentRound),
	}

	png, err := Render(in)
	if err != nil {
		log.Printf("share: render bracket png %s: %v", shortID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	_ = Set(r.Context(), KindBracket, shortID, updatedAt, png)
	writePNG(w, png, shortID, updatedAt)
}

// ---- helpers -------------------------------------------------------

// validShortID lets in only the base62 alphabet we generate with. Stops
// SQL injection shenanigans + truly bizarre inputs from getting to the
// DB layer.
func validShortID(s string) bool {
	if len(s) != 8 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9',
			c >= 'A' && c <= 'Z',
			c >= 'a' && c <= 'z':
			continue
		default:
			return false
		}
	}
	return true
}

func (h *Handler) readPool() *sqlx.DB {
	if h.ReadDB != nil {
		return h.ReadDB
	}
	return h.DB
}

func findMatchupByShortID(ctx context.Context, db *sqlx.DB, shortID string) (*models.Matchup, error) {
	var m models.Matchup
	err := sqlx.GetContext(ctx, db, &m,
		"SELECT * FROM matchups WHERE short_id = $1 LIMIT 1", shortID)
	if err != nil {
		return nil, err
	}
	// Populate the Author embedded struct — we need the username for
	// rendering.
	_ = sqlx.GetContext(ctx, db, &m.Author,
		"SELECT * FROM users WHERE id = $1", m.AuthorID)
	return &m, nil
}

func findBracketByShortID(ctx context.Context, db *sqlx.DB, shortID string) (*models.Bracket, error) {
	var b models.Bracket
	err := sqlx.GetContext(ctx, db, &b,
		"SELECT * FROM brackets WHERE short_id = $1 LIMIT 1", shortID)
	if err != nil {
		return nil, err
	}
	_ = sqlx.GetContext(ctx, db, &b.Author,
		"SELECT * FROM users WHERE id = $1", b.AuthorID)
	return &b, nil
}

func loadMatchupItems(ctx context.Context, db *sqlx.DB, matchupID uint) ([]models.MatchupItem, error) {
	var items []models.MatchupItem
	err := sqlx.SelectContext(ctx, db, &items,
		"SELECT * FROM matchup_items WHERE matchup_id = $1 ORDER BY id ASC", matchupID)
	return items, err
}

func respondNotFoundOr410(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	log.Printf("share: lookup: %v", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

// shouldObscure returns true when a matchup's content must not be
// rendered — i.e. the author is private and we haven't authenticated
// the viewer (and share routes are always unauthenticated).
func shouldObscure(m *models.Matchup) bool {
	if m == nil {
		return true
	}
	return m.Author.IsPrivate || m.Visibility == "private"
}

func bracketShouldObscure(b *models.Bracket) bool {
	if b == nil {
		return true
	}
	return b.Author.IsPrivate || b.Visibility == "private"
}

func matchupPreview(m *models.Matchup, originBase string) Preview {
	shortID := ""
	if m.ShortID != nil {
		shortID = *m.ShortID
	}
	return Preview{
		Kind:        KindMatchup,
		ShortID:     shortID,
		CanonicalID: m.PublicID,
		Title:       m.Title,
		Description: matchupDescription(m),
		AuthorName:  authorName(m.Author.Username),
		UpdatedAt:   m.UpdatedAt,
		SPAPath:     matchupSPAPath(m),
	}
}

func bracketPreview(b *models.Bracket, originBase string) Preview {
	shortID := ""
	if b.ShortID != nil {
		shortID = *b.ShortID
	}
	return Preview{
		Kind:        KindBracket,
		ShortID:     shortID,
		CanonicalID: b.PublicID,
		Title:       b.Title,
		Description: bracketDescription(b),
		AuthorName:  authorName(b.Author.Username),
		UpdatedAt:   b.UpdatedAt,
		SPAPath:     bracketSPAPath(b),
	}
}

func matchupDescription(m *models.Matchup) string {
	author := authorName(m.Author.Username)
	if author != "" {
		return fmt.Sprintf("Vote on this matchup by @%s on Matchup.", author)
	}
	return "Vote on this matchup on Matchup."
}

func bracketDescription(b *models.Bracket) string {
	author := authorName(b.Author.Username)
	if author != "" {
		return fmt.Sprintf("Tournament bracket by @%s on Matchup.", author)
	}
	return "Tournament bracket on Matchup."
}

func matchupSPAPath(m *models.Matchup) string {
	uid := m.Author.Username
	if uid == "" {
		uid = m.Author.PublicID
	}
	if uid == "" {
		uid = "u"
	}
	return "/users/" + url.PathEscape(uid) + "/matchup/" + m.PublicID
}

func bracketSPAPath(b *models.Bracket) string {
	return "/brackets/" + b.PublicID
}

func authorName(s string) string {
	return strings.TrimSpace(s)
}

// writePNG writes a ready-made PNG body with the right headers. The
// ETag is weak because we consider pixel-identical regenerations
// "equivalent"; the strong invariant we care about is content matching
// the (short_id, updated_at) pair.
func writePNG(w http.ResponseWriter, body []byte, shortID string, updatedAtUnix int64) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Access-Control-Allow-Origin", "*") // Twitter inline fetch
	w.Header().Set("Cache-Control", "public, max-age=86400, s-maxage=604800, stale-while-revalidate=86400")
	w.Header().Set("ETag", fmt.Sprintf(`W/"%s-%d"`, shortID, updatedAtUnix))
	_, _ = w.Write(body)
}

// writeFallbackPNG emits a generic "Matchup" card when the short_id
// doesn't resolve. Never 404 — crawlers aggressively cache 404s and
// leaving them that way means a future recreate can't unfurl.
func (h *Handler) writeFallbackPNG(w http.ResponseWriter, r *http.Request, shortID string, kind ContentKind) {
	png, err := Render(ImageInput{
		Kind:       kind,
		Title:      "Matchup",
		AuthorName: "",
		ShareURL:   h.originFromRequest(r),
	})
	if err != nil {
		log.Printf("share: fallback render: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	// Shorter cache — if the user recreates with the same short_id
	// (unlikely) we want fresh data.
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(png)
}

// writeObscuredPNG emits a privacy-safe card (no title, no items) when
// the underlying author is private. Shown regardless of cache state.
func (h *Handler) writeObscuredPNG(w http.ResponseWriter, r *http.Request, kind ContentKind) {
	png, err := Render(ImageInput{
		Kind:       kind,
		Title:      "Private on Matchup",
		AuthorName: "",
		ShareURL:   h.originFromRequest(r),
	})
	if err != nil {
		log.Printf("share: obscured render: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=60")
	_, _ = w.Write(png)
}

