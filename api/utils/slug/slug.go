// Package slug provides community-slug validation + suggestion helpers.
//
// Slugs are immutable after creation (URL stability), so we want to be
// strict at create time: lowercase alphanumeric + hyphens, no leading/
// trailing hyphen, no doubled hyphens, length 3-32. Plus a reserved
// word list so users can't create /c/admin, /c/api, etc.
package slug

import (
	"regexp"
	"strings"
)

const (
	MinLength = 3
	MaxLength = 32
)

// reservedSlugs are paths/words we never let a community claim. Some
// reflect route collisions (/c/new would conflict with the create
// page); others block impersonation (/c/admin, /c/official). Lowercase
// only — slugs are normalized to lowercase before this check.
var reservedSlugs = map[string]bool{
	// Route collisions on /c/<slug>
	"new":        true,
	"create":     true,
	"edit":       true,
	"settings":   true,
	"about":      true,
	"members":    true,
	"mod":        true,
	"mods":       true,
	"admin":      true,
	"api":        true,

	// Top-level path collisions (defensive — even though /c/ is its
	// own namespace, future refactors might collapse it)
	"home":       true,
	"login":      true,
	"register":   true,
	"signup":     true,
	"signin":     true,
	"profile":    true,
	"user":       true,
	"users":      true,
	"matchup":    true,
	"matchups":   true,
	"bracket":    true,
	"brackets":   true,
	"community":  true,
	"communities":true,

	// Brand / impersonation
	"matchup-team":    true,
	"matchup-staff":   true,
	"matchup-admin":   true,
	"official":        true,
	"support":         true,
	"help":            true,
	"team":            true,
	"staff":           true,
	"moderator":       true,
	"administrator":   true,
}

// validPattern enforces lowercase alphanumeric + hyphens with no
// leading/trailing hyphen and no double hyphens. Pre-compiled at
// package init so the hot CheckSlug path doesn't reparse.
var validPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9]|-(?:[a-z0-9]))*$`)

// Reason values returned alongside Available=false.
const (
	ReasonInvalid  = "invalid"
	ReasonReserved = "reserved"
)

// Normalize lowercases + trims whitespace. Use at the API boundary
// (CreateCommunity, CheckSlugAvailable) before any other check.
func Normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// Validate returns "" if the slug is structurally valid, or a Reason
// string explaining why not. Does NOT check uniqueness — that's a DB
// concern. Assumes the input is already normalized.
func Validate(s string) string {
	if len(s) < MinLength || len(s) > MaxLength {
		return ReasonInvalid
	}
	if !validPattern.MatchString(s) {
		return ReasonInvalid
	}
	if reservedSlugs[s] {
		return ReasonReserved
	}
	return ""
}

// IsReserved exposes the reserved-word check for callers that need
// to validate against the same list outside the normal flow.
func IsReserved(s string) bool {
	return reservedSlugs[s]
}
