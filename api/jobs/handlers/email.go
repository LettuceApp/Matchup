// Package handlers contains the per-queue Handler implementations
// consumed by api/cmd/worker. They live in their own package so the
// worker binary depends on as little as possible — pulling in the full
// controllers package would make the worker image massive and create a
// circular dependency since controllers itself enqueues jobs.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"

	"Matchup/mailer"
)

// EmailQueue is the queue name producers must use to enqueue email
// payloads. Kept as a constant so the producer side and worker side
// can't drift apart silently.
const EmailQueue = "email"

// EmailKindResetPassword is the only template currently routed through
// the queue. Adding new kinds is just a matter of adding another
// constant + handler branch.
const EmailKindResetPassword = "reset_password"

// EmailJob is the JSON payload format the email worker expects on the
// wire. The producer marshals one of these and calls
// jobs.Enqueue(ctx, EmailQueue, payload).
//
// Keeping this struct in a shared package (rather than duplicating it
// inside the controller) is important: a typo on either side silently
// drops fields.
type EmailJob struct {
	Kind  string `json:"kind"`
	To    string `json:"to"`
	Token string `json:"token,omitempty"`
}

// HandleEmail decodes an EmailJob from the queue and dispatches to the
// matching mailer call. Errors come back as ordinary error values; the
// worker logs them and moves on (no retry — see jobs.RunWorker).
func HandleEmail(ctx context.Context, payload []byte) error {
	var job EmailJob
	if err := json.Unmarshal(payload, &job); err != nil {
		return err
	}

	switch job.Kind {
	case EmailKindResetPassword:
		fromAdmin := os.Getenv("ADMIN_EMAIL")
		sendgridKey := os.Getenv("SENDGRID_API_KEY")
		appEnv := os.Getenv("APP_ENV")
		resp, err := mailer.SendMail.SendResetPassword(job.To, fromAdmin, job.Token, sendgridKey, appEnv)
		if err != nil {
			return err
		}
		log.Printf("email worker: reset password sent to %s (status=%d)", job.To, resp.Status)
		return nil
	default:
		return errors.New("email worker: unknown kind " + job.Kind)
	}
}
