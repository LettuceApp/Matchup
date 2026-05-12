package mailer

import (
	"log"
	"os"
	"strings"
)

// genericSenderDomains is the set of well-known consumer/free email
// providers and SendGrid's own helper domain. Using any of these as
// SENDGRID_FROM means Gmail will display the email as "via sendgrid.net"
// (or worse, drop it into spam) because DKIM can't be aligned to a
// domain that we don't own.
var genericSenderDomains = []string{
	"@gmail.com",
	"@yahoo.com",
	"@outlook.com",
	"@hotmail.com",
	"@aol.com",
	"@icloud.com",
	"@sendgrid.net",
	"@em.sendgrid.net",
}

// CheckSenderConfig logs a warning at startup when SENDGRID_FROM is
// missing or set to an address on a domain we cannot DKIM-align with
// SendGrid. Doesn't block boot — mail will still attempt to go out;
// users will just see "via sendgrid.net" labels and spam-folder
// deliveries until the operator points SENDGRID_FROM at a domain
// that's authenticated via SendGrid Sender Authentication (CNAMEs).
//
// Call this once from server.Run() after env vars are loaded. The
// matching DNS playbook lives in api/mailer/SENDGRID_SETUP.md.
func CheckSenderConfig() {
	from := strings.TrimSpace(os.Getenv("SENDGRID_FROM"))
	if from == "" {
		log.Printf("[mailer] WARNING: SENDGRID_FROM is empty — verification, password-reset, and digest emails will fail. Set it to a verified Single Sender or, better, a noreply@ address on a domain authenticated with SendGrid.")
		return
	}

	lower := strings.ToLower(from)
	for _, d := range genericSenderDomains {
		if strings.HasSuffix(lower, d) {
			log.Printf(
				"[mailer] WARNING: SENDGRID_FROM=%q is on a generic/unauthenticated domain (%s) — Gmail will label these emails 'via sendgrid.net' and many providers will spam-foldering them. To fix, complete SendGrid Domain Authentication on your own domain (api/mailer/SENDGRID_SETUP.md) and set SENDGRID_FROM=noreply@<your-domain>.",
				from, strings.TrimPrefix(d, "@"),
			)
			return
		}
	}
}
