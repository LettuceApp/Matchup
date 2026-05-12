package mailer

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/matcornic/hermes/v2"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

// DigestPayload is the composed view-model for one user's weekly email.
// Kept kind-neutral: the controller-side composer fills it in with
// counts + a small set of highlight items. The mailer renders it into
// HTML via hermes.
//
// All integer fields default to 0; a payload where every count is 0
// AND HighlightItems is empty should NOT be sent — the composer skips
// those users rather than shipping an "all quiet" email.
type DigestPayload struct {
	// ToUser is the recipient's display name / username — shown in the
	// greeting and nowhere else.
	ToUser string
	// ToEmail is the recipient's email address.
	ToEmail string
	// Signed unsubscribe URL (absolute). Rendered as a plain link in
	// the footer, matching CAN-SPAM / best-practice guidance.
	UnsubscribeURL string

	// Headline counts. Zero-valued counts are simply omitted from the
	// rendered HTML so the email stays short for quiet weeks.
	NewLikes        int
	NewComments     int
	NewFollowers    int
	NewMentions     int
	NewMilestones   int
	ClosingSoon     int

	// HighlightItems are the top N "you might want to look at these"
	// entries — typically posts from followed creators or highly-engaged
	// matchups you authored. Each entry renders as a bullet with a
	// title + the canonical SPA link.
	HighlightItems []DigestHighlight
}

// DigestHighlight is one row in the digest's highlight list.
type DigestHighlight struct {
	Title string // short, 50ish chars
	URL   string // absolute SPA link
	// Optional subtitle — "@author posted this" or "won with 32 votes"
	// — keeps the bullet readable without a second mental jump.
	Subtitle string
}

// AnythingToReport returns true when the payload has at least one
// headline count OR at least one highlight item. Composer uses this
// to skip truly-empty weeks.
func (p DigestPayload) AnythingToReport() bool {
	if p.NewLikes+p.NewComments+p.NewFollowers+p.NewMentions+p.NewMilestones+p.ClosingSoon > 0 {
		return true
	}
	return len(p.HighlightItems) > 0
}

// DigestMailer is the interface the scheduler uses so tests can swap
// in a capture-only fake. The real implementation is sendMail{} (same
// value used by the password-reset mailer); the interface intentionally
// returns the same EmailResponse shape for consistency.
type DigestMailer interface {
	SendWeeklyDigest(payload DigestPayload, fromAdmin, sendgridKey string) (*EmailResponse, error)
}

// DigestMail is the shared singleton. Tests can overwrite it with a
// fake that records payloads instead of hitting SendGrid.
var DigestMail DigestMailer = &sendMail{}

// SendWeeklyDigest composes a hermes email from a DigestPayload and
// ships it via SendGrid. Returns (200, "OK") on successful dispatch;
// any SendGrid error bubbles up.
func (s *sendMail) SendWeeklyDigest(payload DigestPayload, fromAdmin, sendgridKey string) (*EmailResponse, error) {
	h := hermes.Hermes{
		Product: hermes.Product{
			Name:      "Matchup",
			Link:      appBaseURL(),
			Copyright: "Copyright © Matchup",
		},
	}

	body := hermes.Body{
		Name:   payload.ToUser,
		Intros: []string{buildDigestIntro(payload)},
	}

	// Stats block — only rendered when at least one count is nonzero.
	// Uses a simple dictionary so hermes lays it out as a key/value
	// table rather than forcing us into prose for every line.
	stats := buildDigestStats(payload)
	if len(stats) > 0 {
		body.Dictionary = stats
	}

	// Highlight bullets render as a hermes table. Empty HighlightItems
	// means no table is included, which is fine — stats carry the news.
	if len(payload.HighlightItems) > 0 {
		body.Table = hermes.Table{
			Data: buildDigestTable(payload.HighlightItems),
			Columns: hermes.Columns{
				CustomWidth: map[string]string{
					"Title":    "70%",
					"Subtitle": "30%",
				},
			},
		}
	}

	body.Actions = []hermes.Action{
		{
			Instructions: "Jump back in:",
			Button: hermes.Button{
				// See verify_email_mail.go for the full note — hermes
				// paints white text on whatever Color we set, so #FFFFFF
				// rendered an invisible button.
				Color: "#F97316",
				Text:  "Open Matchup",
				Link:  appBaseURL(),
			},
		},
	}

	// Footer carries the opt-out link. Hermes renders Outros as plain
	// paragraphs — exactly what a compliant unsubscribe footer needs.
	body.Outros = []string{
		"You're receiving this weekly digest because email notifications are enabled on your account.",
		fmt.Sprintf("To stop these emails, click here: %s", payload.UnsubscribeURL),
	}

	email := hermes.Email{Body: body}
	htmlBody, err := h.GenerateHTML(email)
	if err != nil {
		return nil, err
	}

	from := mail.NewEmail("Matchup", fromAdmin)
	subject := "Your weekly Matchup digest"
	to := mail.NewEmail(payload.ToUser, payload.ToEmail)
	message := mail.NewSingleEmail(from, subject, to, htmlBody, htmlBody)

	client := sendgrid.NewSendClient(sendgridKey)
	if _, err := client.Send(message); err != nil {
		return nil, err
	}

	return &EmailResponse{
		Status:   http.StatusOK,
		RespBody: "digest sent",
	}, nil
}

func buildDigestIntro(p DigestPayload) string {
	if !p.AnythingToReport() {
		// Defensive — composer shouldn't call the mailer on empty
		// payloads, but keep the intro sensible anyway.
		return "Here's your weekly Matchup recap — quiet week!"
	}
	return "Here's what's happened on Matchup since your last digest."
}

func buildDigestStats(p DigestPayload) []hermes.Entry {
	add := func(out []hermes.Entry, label string, n int) []hermes.Entry {
		if n <= 0 {
			return out
		}
		return append(out, hermes.Entry{Key: label, Value: strconv.Itoa(n)})
	}
	var e []hermes.Entry
	e = add(e, "New likes on your posts", p.NewLikes)
	e = add(e, "New comments", p.NewComments)
	e = add(e, "New followers", p.NewFollowers)
	e = add(e, "Mentions", p.NewMentions)
	e = add(e, "Milestones hit", p.NewMilestones)
	e = add(e, "Closing soon", p.ClosingSoon)
	return e
}

func buildDigestTable(items []DigestHighlight) [][]hermes.Entry {
	out := make([][]hermes.Entry, 0, len(items))
	for _, h := range items {
		// The Title cell is a markdown link so hermes renders a blue
		// clickable title. Subtitle stays plain text.
		titleCell := fmt.Sprintf("[%s](%s)", escapeMarkdown(h.Title), h.URL)
		out = append(out, []hermes.Entry{
			{Key: "Title", Value: titleCell},
			{Key: "Subtitle", Value: h.Subtitle},
		})
	}
	return out
}

// escapeMarkdown keeps titles containing `]` or `[` from breaking the
// hermes markdown renderer. Keeps it simple — no need for a full
// escaper.
func escapeMarkdown(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `[`, `\[`)
	s = strings.ReplaceAll(s, `]`, `\]`)
	return s
}

// appBaseURL returns the canonical front-end URL. Matches the logic in
// the password-reset mailer so dev and prod both get sensible links.
func appBaseURL() string {
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
