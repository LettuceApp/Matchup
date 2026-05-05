// Package sentry is a thin wrapper around the official sentry-go SDK.
// It centralises initialisation + graceful shutdown + environment
// resolution so the rest of the codebase only calls `sentry.Init()`
// at boot and `sentry.CaptureException(err)` at the useful moments.
//
// Design intent: zero-config in dev (Sentry stays off when SENTRY_DSN
// is unset) and opt-in per-env in staging/prod.
package sentry

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/getsentry/sentry-go"
)

// Init spins up the Sentry SDK if SENTRY_DSN is set. Returns a
// flushFn the caller can defer to drain pending events before exit —
// crash reports submitted right before process exit would otherwise
// get dropped when the HTTP client shuts down mid-flight.
//
// Env vars:
//   SENTRY_DSN            — the project DSN (from sentry.io). Empty → no-op.
//   SENTRY_ENVIRONMENT    — "development" | "staging" | "production". Defaults to APP_ENV.
//   SENTRY_RELEASE        — release identifier, typically the git SHA. Passed in at build
//                           time via -ldflags so the binary knows its own version.
//   SENTRY_TRACES_SAMPLE  — trace sample rate 0.0..1.0. Defaults to 0.1 in prod, 1.0 elsewhere.
//
// The flush function is always safe to call; it's a no-op when Sentry
// wasn't initialised.
func Init() func() {
	dsn := os.Getenv("SENTRY_DSN")
	if dsn == "" {
		log.Println("sentry: SENTRY_DSN empty, error reporting disabled")
		return func() {}
	}

	env := os.Getenv("SENTRY_ENVIRONMENT")
	if env == "" {
		env = os.Getenv("APP_ENV")
	}
	if env == "" {
		env = "development"
	}

	release := os.Getenv("SENTRY_RELEASE")

	sampleRate := defaultSampleRate(env)
	if raw := os.Getenv("SENTRY_TRACES_SAMPLE"); raw != "" {
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil && parsed >= 0 && parsed <= 1 {
			sampleRate = parsed
		}
	}

	if err := sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Environment:      env,
		Release:          release,
		TracesSampleRate: sampleRate,
		// Attach stack traces to captureMessage calls too — useful for
		// non-panic "something bad happened" reports we sprinkle into
		// the handler error branches.
		AttachStacktrace: true,
	}); err != nil {
		log.Printf("sentry: init failed: %v (continuing without reporting)", err)
		return func() {}
	}

	log.Printf("sentry: initialised (env=%s, release=%s, sample=%.2f)", env, release, sampleRate)
	return func() {
		sentry.Flush(2 * time.Second)
	}
}

// defaultSampleRate returns the sensible default for the given
// environment. Prod uses 10% so the free tier lasts; staging is
// 100% so we never miss a repro of a staging-only bug; dev is 100%
// (but the DSN is typically empty anyway).
func defaultSampleRate(env string) float64 {
	if env == "production" {
		return 0.1
	}
	return 1.0
}

// CaptureError is a thin helper so callsites don't need to import
// sentry-go directly. Safe to call when Sentry isn't initialised.
func CaptureError(err error) {
	if err == nil {
		return
	}
	sentry.CaptureException(err)
}

// CaptureMessage logs a plain string without a Go error value. Used
// for "worth investigating" conditions that aren't errors per se —
// e.g. a third-party returned 200 but the body was empty.
func CaptureMessage(msg string) {
	sentry.CaptureMessage(msg)
}
