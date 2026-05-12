package mailer

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/matcornic/hermes/v2"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

type sendMail struct{}

type SendMailer interface {
	SendResetPassword(string, string, string, string, string) (*EmailResponse, error)
	SendVerifyEmail(string, string, string, string, string) (*EmailResponse, error)
}

var (
	SendMail SendMailer = &sendMail{} //this is useful when we start testing
)

type EmailResponse struct {
	Status   int
	RespBody string
}

// resetPasswordBaseURL returns the origin the password-reset link
// should point at. Priority: explicit PUBLIC_ORIGIN env (set in prod
// + staging so the link domain matches the deployment), then
// production default, then local-dev default. Keeping this in one
// place means every new mailer template reuses the same resolution
// logic — see the email-verification mailer added in a later phase.
func resetPasswordBaseURL() string {
	if origin := strings.TrimRight(os.Getenv("PUBLIC_ORIGIN"), "/"); origin != "" {
		return origin
	}
	if os.Getenv("APP_ENV") == "production" {
		return "https://matchup.com"
	}
	return "http://127.0.0.1:3000"
}

func (s *sendMail) SendResetPassword(ToUser string, FromAdmin string, Token string, Sendgridkey string, AppEnv string) (*EmailResponse, error) {
	h := hermes.Hermes{
		Product: hermes.Product{
			Name: "Matchup",
			Link: resetPasswordBaseURL(),
		},
	}
	// React route lives at /reset-password/:token (dashed — matches
	// the rest of the frontend route convention). The old
	// /resetpassword/:token URL is not registered, so users whose
	// emails predated this change would've hit a 404.
	forgotUrl := resetPasswordBaseURL() + "/reset-password/" + Token
	email := hermes.Email{
		Body: hermes.Body{
			Name: ToUser,
			Intros: []string{
				"Someone requested a password reset for your Matchup account.",
				"If that wasn't you, you can safely ignore this email — your password hasn't changed.",
			},
			Actions: []hermes.Action{
				{
					Instructions: "Click this link to reset your password. It expires in 2 hours.",
					Button: hermes.Button{
						// See verify_email_mail.go for the full note —
						// hermes uses Color as the button background and
						// always paints white text, so #FFFFFF rendered
						// the button invisible against the email body.
						Color: "#F97316",
						Text:  "Reset Password",
						Link:  forgotUrl,
					},
				},
			},
			Outros: []string{
				"Need help, or have questions? Just reply to this email — we'd love to help.",
			},
		},
	}
	emailBody, err := h.GenerateHTML(email)
	if err != nil {
		return nil, err
	}
	from := mail.NewEmail("Matchup", FromAdmin)
	subject := "Reset your Matchup password"
	to := mail.NewEmail(ToUser, ToUser)
	message := mail.NewSingleEmail(from, subject, to, emailBody, emailBody)
	client := sendgrid.NewSendClient(Sendgridkey)
	resp, err := client.Send(message)
	if err != nil {
		return nil, err
	}
	// Same SendGrid-status check as SendVerifyEmail — non-2xx means
	// the API rejected the send (bad key, unverified sender, etc.)
	// even though the HTTP transport succeeded.
	if resp == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := ""
		status := 0
		if resp != nil {
			body = resp.Body
			status = resp.StatusCode
		}
		log.Printf("mailer: SendGrid rejected reset-password email (status %d): %s", status, body)
		return nil, fmt.Errorf("sendgrid status %d: %s", status, body)
	}
	return &EmailResponse{
		Status:   http.StatusOK,
		RespBody: "Success, Please click on the link provided in your email",
	}, nil
}
