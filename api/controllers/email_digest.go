package controllers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"Matchup/mailer"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// digestCooldown prevents double-sending. The weekly scheduler fires
// once per week, but a crash mid-batch + retry or an on-demand manual
// run shouldn't double-bill an inbox. 6 days is safe even with minor
// schedule drift; 7 would risk skipping a send.
const digestCooldown = 6 * 24 * time.Hour

// digestHighlightLimit caps the highlight-bullet list in each email
// to keep the inbox preview short. 3 hits the sweet spot for CTA-level
// nudges without feeling like a feed.
const digestHighlightLimit = 3

// digestCandidate is a one-row view we pull from the users table when
// scanning for eligible recipients. Keeps the per-user per-batch work
// fast — we only hydrate the full User model when we've decided to
// send to that row.
type digestCandidate struct {
	ID                    uint       `db:"id"`
	Username              string     `db:"username"`
	Email                 string     `db:"email"`
	NotificationPrefs     []byte     `db:"notification_prefs"`
	EmailDigestLastSentAt *time.Time `db:"email_digest_last_sent_at"`
}

// SendEmailDigests scans users with email_digest enabled and whose
// last send was outside the cooldown window, composes per-user
// payloads, and ships each via the configured mailer. Returns the
// number of emails actually sent (skipping empty weeks + already-sent
// users).
//
// Intended call sites:
//   - scheduler.go Sunday 14:00 UTC cron
//   - admin "re-run digest" trigger (future; not wired yet)
//
// Mailer is taken from mailer.DigestMail so tests can substitute a
// capture-only fake without hitting SendGrid.
func SendEmailDigests(ctx context.Context, db *sqlx.DB) (int, error) {
	fromAdmin := os.Getenv("SENDGRID_FROM")
	sendgridKey := os.Getenv("SENDGRID_API_KEY")
	if fromAdmin == "" || sendgridKey == "" {
		return 0, fmt.Errorf("email_digest: SENDGRID_FROM or SENDGRID_API_KEY not set")
	}

	cutoff := time.Now().Add(-digestCooldown)

	var candidates []digestCandidate
	if err := sqlx.SelectContext(ctx, db, &candidates, `
		SELECT id, username, email, notification_prefs, email_digest_last_sent_at
		  FROM public.users
		 WHERE (notification_prefs ->> 'email_digest') = 'true'
		   AND (email_digest_last_sent_at IS NULL OR email_digest_last_sent_at < $1)
	`, cutoff); err != nil {
		return 0, fmt.Errorf("email_digest: select candidates: %w", err)
	}

	sent := 0
	for _, u := range candidates {
		payload, err := composeDigestFor(ctx, db, u)
		if err != nil {
			log.Printf("email_digest: compose for user %d failed: %v", u.ID, err)
			continue
		}
		if !payload.AnythingToReport() {
			// Quiet week — don't send, don't stamp. The user will be
			// eligible again on the next tick, so if something lands
			// tomorrow they get it in the following Sunday's batch.
			continue
		}
		if _, err := mailer.DigestMail.SendWeeklyDigest(payload, fromAdmin, sendgridKey); err != nil {
			log.Printf("email_digest: send to %s failed: %v", u.Email, err)
			continue
		}
		if _, err := db.ExecContext(ctx,
			`UPDATE public.users SET email_digest_last_sent_at = NOW() WHERE id = $1`,
			u.ID,
		); err != nil {
			// The email already went out; a stamp failure means we might
			// re-send next tick (within cooldown). Log and move on.
			log.Printf("email_digest: stamp for user %d failed: %v", u.ID, err)
			continue
		}
		sent++
	}
	return sent, nil
}

// composeDigestFor gathers the counts + highlights for a single user
// since their last digest (or the last 7 days if first-time).
func composeDigestFor(ctx context.Context, db *sqlx.DB, u digestCandidate) (mailer.DigestPayload, error) {
	payload := mailer.DigestPayload{
		ToUser:         u.Username,
		ToEmail:        u.Email,
		UnsubscribeURL: BuildUnsubscribeURL(u.ID),
	}

	since := time.Now().Add(-7 * 24 * time.Hour)
	if u.EmailDigestLastSentAt != nil && u.EmailDigestLastSentAt.After(since) {
		since = *u.EmailDigestLastSentAt
	}

	// Likes received (owner-excluded, matching the activity-feed query).
	if err := db.GetContext(ctx, &payload.NewLikes, `
		SELECT COUNT(*) FROM public.likes l
		  JOIN public.matchups m ON m.id = l.matchup_id
		 WHERE m.author_id = $1
		   AND l.user_id <> $1
		   AND l.created_at >= $2
	`, u.ID, since); err != nil {
		return payload, fmt.Errorf("count likes: %w", err)
	}

	// Comments received (owner-excluded).
	if err := db.GetContext(ctx, &payload.NewComments, `
		SELECT COUNT(*) FROM public.comments c
		  JOIN public.matchups m ON m.id = c.matchup_id
		 WHERE m.author_id = $1
		   AND c.user_id <> $1
		   AND c.created_at >= $2
	`, u.ID, since); err != nil {
		return payload, fmt.Errorf("count comments: %w", err)
	}

	// New followers since last digest.
	if err := db.GetContext(ctx, &payload.NewFollowers, `
		SELECT COUNT(*) FROM public.follows
		 WHERE followed_id = $1
		   AND created_at >= $2
	`, u.ID, since); err != nil {
		return payload, fmt.Errorf("count followers: %w", err)
	}

	// Mentions + milestones + closing-soon come out of the persisted
	// notifications table. We count unread rows since last digest —
	// if the user already opened the bell during the week and stamped
	// them read, they're excluded (correct: they've already seen it).
	if err := db.GetContext(ctx, &payload.NewMentions, `
		SELECT COUNT(*) FROM public.notifications
		 WHERE user_id = $1 AND kind = 'mention_received'
		   AND occurred_at >= $2
	`, u.ID, since); err != nil {
		return payload, fmt.Errorf("count mentions: %w", err)
	}
	if err := db.GetContext(ctx, &payload.NewMilestones, `
		SELECT COUNT(*) FROM public.notifications
		 WHERE user_id = $1 AND kind = 'milestone_reached'
		   AND occurred_at >= $2
	`, u.ID, since); err != nil {
		return payload, fmt.Errorf("count milestones: %w", err)
	}
	if err := db.GetContext(ctx, &payload.ClosingSoon, `
		SELECT COUNT(*) FROM public.notifications
		 WHERE user_id = $1
		   AND kind IN ('matchup_closing_soon', 'tie_needs_resolution')
		   AND occurred_at >= $2
	`, u.ID, since); err != nil {
		return payload, fmt.Errorf("count closing: %w", err)
	}

	// Highlights: top N new posts from followed creators since last
	// digest. Matches the read-path query shape in
	// loadFollowedUserPostedActivity (same joins, same visibility
	// filters) but limited to the digest window.
	type highlightRow struct {
		Title          string    `db:"title"`
		PublicID       string    `db:"public_id"`
		SubjectType    string    `db:"subject_type"`
		AuthorUsername string    `db:"author_username"`
		CreatedAt      time.Time `db:"created_at"`
	}
	var highlights []highlightRow
	if err := sqlx.SelectContext(ctx, db, &highlights, `
		SELECT * FROM ((
		  SELECT m.title, m.public_id, 'matchup' AS subject_type, u.username AS author_username, m.created_at
		  FROM public.matchups m
		  JOIN public.follows f ON f.followed_id = m.author_id AND f.follower_id = $1
		  JOIN public.users u   ON u.id = m.author_id
		  WHERE m.author_id <> $1
		    AND m.status IN ('active', 'published', 'completed')
		    AND m.bracket_id IS NULL
		    AND m.created_at >= $2
		) UNION ALL (
		  SELECT b.title, b.public_id, 'bracket' AS subject_type, u.username AS author_username, b.created_at
		  FROM public.brackets b
		  JOIN public.follows f ON f.followed_id = b.author_id AND f.follower_id = $1
		  JOIN public.users u   ON u.id = b.author_id
		  WHERE b.author_id <> $1
		    AND b.status IN ('active', 'published', 'completed')
		    AND b.created_at >= $2
		)) t ORDER BY created_at DESC LIMIT $3
	`, u.ID, since, digestHighlightLimit); err != nil {
		return payload, fmt.Errorf("select highlights: %w", err)
	}
	for _, h := range highlights {
		payload.HighlightItems = append(payload.HighlightItems, mailer.DigestHighlight{
			Title:    h.Title,
			URL:      buildContentURL(h.SubjectType, h.PublicID, h.AuthorUsername),
			Subtitle: "@" + h.AuthorUsername,
		})
	}

	return payload, nil
}

// buildContentURL mirrors the frontend's subjectPath function for the
// two kinds we surface in highlights. Matchup links need the author
// username in the path; brackets use a flat /brackets/:id route.
func buildContentURL(subjectType, publicID, authorUsername string) string {
	base := strings.TrimRight(mailerAppBaseURL(), "/")
	switch subjectType {
	case "matchup":
		return fmt.Sprintf("%s/users/%s/matchup/%s", base, authorUsername, publicID)
	case "bracket":
		return fmt.Sprintf("%s/brackets/%s", base, publicID)
	default:
		return base
	}
}

// mailerAppBaseURL reads the same env shape as the mailer so links in
// the controller stay in sync with link rendering.
func mailerAppBaseURL() string {
	if os.Getenv("APP_ENV") == "production" {
		if u := os.Getenv("FRONTEND_ORIGIN"); u != "" {
			return u
		}
		return "https://matchup.com"
	}
	if u := os.Getenv("FRONTEND_ORIGIN"); u != "" {
		return u
	}
	return "http://127.0.0.1:3000"
}

// --------------------------------------------------------------------
// One-click unsubscribe — HMAC-signed token, stateless, link lands on
// a public chi route that sets email_digest=false and redirects to a
// small confirmation page.
// --------------------------------------------------------------------

// BuildUnsubscribeURL returns an absolute URL carrying an HMAC-signed
// token that encodes (user_id, "email_digest"). Exported so the mailer
// can build links without duplicating the signing logic.
func BuildUnsubscribeURL(userID uint) string {
	token := signUnsubscribeToken(userID)
	base := strings.TrimRight(publicAPIBaseURL(), "/")
	return fmt.Sprintf("%s/email/unsubscribe/%s", base, token)
}

// signUnsubscribeToken returns "<uid>.<hmac_hex>" where the HMAC is
// computed over "<uid>:email_digest" with API_SECRET as the key.
// Short-form binary tokens would be nicer but base16 keeps this
// copy-pasteable from email clients that mangle URL-safe base64.
func signUnsubscribeToken(userID uint) string {
	secret := []byte(os.Getenv("API_SECRET"))
	uidStr := strconv.FormatUint(uint64(userID), 10)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(uidStr + ":email_digest"))
	return uidStr + "." + hex.EncodeToString(mac.Sum(nil))
}

// verifyUnsubscribeToken returns the encoded userID when the signature
// matches, or 0 + error otherwise. Constant-time compare prevents
// timing leaks on the HMAC.
func verifyUnsubscribeToken(token string) (uint, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("bad token shape")
	}
	uidInt, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bad uid: %w", err)
	}
	expected := signUnsubscribeToken(uint(uidInt))
	if !hmac.Equal([]byte(expected), []byte(token)) {
		return 0, fmt.Errorf("signature mismatch")
	}
	return uint(uidInt), nil
}

// publicAPIBaseURL returns the origin clients call — used when building
// the unsubscribe link because the route lives on the API service, not
// the frontend.
func publicAPIBaseURL() string {
	if u := os.Getenv("PUBLIC_ORIGIN"); u != "" {
		return u
	}
	return "http://127.0.0.1:8888"
}

// EmailUnsubscribeHandler handles GET /email/unsubscribe/:token. On
// valid token, flips email_digest=false on the user's notification
// prefs and returns a tiny HTML confirmation page. Invalid tokens get
// a 400 with a generic message (don't leak HMAC detail).
//
// Route is auth-optional — the token IS the auth. A recipient forwarding
// the email to a friend can't unsubscribe them because the HMAC over
// the uid won't match.
func EmailUnsubscribeHandler(db *sqlx.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")
		uid, err := verifyUnsubscribeToken(token)
		if err != nil {
			http.Error(w, "This unsubscribe link is invalid or expired.", http.StatusBadRequest)
			return
		}
		if _, err := db.ExecContext(r.Context(), `
			UPDATE public.users
			   SET notification_prefs = jsonb_set(
			       COALESCE(notification_prefs, '{}'::jsonb),
			       '{email_digest}',
			       'false'::jsonb,
			       true
			   )
			 WHERE id = $1`, uid,
		); err != nil {
			log.Printf("email_digest: unsubscribe update for user %d failed: %v", uid, err)
			http.Error(w, "Sorry, something went wrong.", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, unsubscribeConfirmPage, mailerAppBaseURL())
	}
}

const unsubscribeConfirmPage = `<!doctype html>
<html>
  <head><meta charset="utf-8"><title>Unsubscribed</title></head>
  <body style="font-family: -apple-system, Segoe UI, sans-serif; max-width: 520px; margin: 4rem auto; color: #222; line-height: 1.5;">
    <h1>You're unsubscribed.</h1>
    <p>We won't send you any more weekly digests. You can re-enable them anytime from your notification settings on Matchup.</p>
    <p><a href="%s">Back to Matchup →</a></p>
  </body>
</html>`
