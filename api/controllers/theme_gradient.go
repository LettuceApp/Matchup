package controllers

import "fmt"

// allowedThemeGradients is the wire-side allow-list of curated
// gradient slugs that owners (communities) and users (profiles) can
// pick from. The raw CSS gradient values live on the frontend in
// utils/communityGradients.js — duplicating them here would force a
// backend deploy every time we tweak a color. What MUST stay in
// sync is the slug list: anything not in this set is rejected at
// write-time so a malicious client can't store arbitrary strings
// (which would then flow into inline-style attributes on rendered
// pages).
//
// Empty string is also valid and means "no theme chosen" — the
// frontend falls back to its default palette.
var allowedThemeGradients = map[string]struct{}{
	"":         {}, // explicit "clear theme"
	"stardust": {},
	"sunset":   {},
	"ocean":    {},
	"mint":     {},
	"amber":    {},
	"magenta":  {},
	"forest":   {},
	"plum":     {},
	"rose":     {},
	"graphite": {},
}

// validateThemeGradient rejects unknown slugs with an
// InvalidArgument-style error so callers can wrap it for the
// connect.NewError InvalidArgument code. Used by both the community
// handler (UpdateCommunity) and the user handler (UpdateUser).
func validateThemeGradient(slug string) error {
	if _, ok := allowedThemeGradients[slug]; !ok {
		return fmt.Errorf("unknown theme gradient %q", slug)
	}
	return nil
}
