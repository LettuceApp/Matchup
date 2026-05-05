package mailer

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/matcornic/hermes/v2"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

// SendVerifyEmail is wired into the same `sendMail` struct as
// SendResetPassword so callers use the single `mailer.SendMail`
// entry point. Signature mirrors SendResetPassword on purpose —
// the queue worker dispatches both kinds through near-identical
// branches.
//
// The template stays deliberately short. Apple Mail + Gmail both
// truncate long intros in the preview pane, and this email is the
// user's first onboarding touch — we want "click the button" to be
// the screen's dominant message, not a wall of copy.
func (s *sendMail) SendVerifyEmail(ToUser string, FromAdmin string, Token string, Sendgridkey string, AppEnv string) (*EmailResponse, error) {
	// Loud, actionable error when SendGrid creds are missing. Easy to
	// hit in dev — a fresh .env without SENDGRID_API_KEY would
	// otherwise surface as an opaque "401 Unauthorized" buried in
	// the API logs. The message points at exactly which env var is
	// missing so the fix is one paste away.
	if strings.TrimSpace(Sendgridkey) == "" || strings.TrimSpace(FromAdmin) == "" {
		// Print to stdout AND return an error — both for the inline
		// caller (which logs) and the worker (which logs).
		log.Printf("mailer: cannot send verification email — SENDGRID_API_KEY or SENDGRID_FROM is empty")
		log.Printf("        verify link (paste into the browser to confirm during dev): %s/verify-email/%s",
			resetPasswordBaseURL(), Token)
		return nil, fmt.Errorf("sendgrid creds missing (SENDGRID_API_KEY / SENDGRID_FROM)")
	}

	h := hermes.Hermes{
		Product: hermes.Product{
			Name: "Matchup",
			Link: resetPasswordBaseURL(),
		},
	}
	// Frontend route: /verify-email/:token (listed in AASA exclude
	// so the native app doesn't try to handle it — verification must
	// happen in the browser where the user can see + trust the URL).
	verifyURL := resetPasswordBaseURL() + "/verify-email/" + Token

	email := hermes.Email{
		Body: hermes.Body{
			Name: ToUser,
			Intros: []string{
				"Welcome to Matchup! Tap the button below to verify your email.",
			},
			Actions: []hermes.Action{
				{
					Instructions: "Verify your email so you can start creating matchups and brackets. This link expires in 24 hours.",
					Button: hermes.Button{
						Color: "#FFFFFF",
						Text:  "Verify email",
						Link:  verifyURL,
					},
				},
			},
			Outros: []string{
				"If you didn't create a Matchup account, you can safely ignore this email.",
			},
		},
	}
	emailBody, err := h.GenerateHTML(email)
	if err != nil {
		return nil, err
	}
	from := mail.NewEmail("Matchup", FromAdmin)
	subject := "Verify your email for Matchup"
	to := mail.NewEmail(ToUser, ToUser)
	message := mail.NewSingleEmail(from, subject, to, emailBody, emailBody)
	client := sendgrid.NewSendClient(Sendgridkey)
	if _, err := client.Send(message); err != nil {
		return nil, err
	}
	return &EmailResponse{
		Status:   http.StatusOK,
		RespBody: "verification email sent",
	}, nil
}
