package controllers

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

// Well-known deep-link manifests served at /.well-known/... URLs so
// Apple and Google can dispatch https://matchup.com/... links to the
// installed native app rather than the browser.
//
// Both files are produced at request time from env vars — no static
// files on disk. This keeps dev / staging / prod able to run
// different app identifiers without a file swap and means adding a
// new deep-linkable path is a one-line code change (see
// universalLinkComponents below) with a matching unit test.
//
// Graceful no-op: when the required env vars aren't set, both
// handlers return 404 with a plain-text body. Dev machines without
// iOS/Android config never serve garbage, and prod misconfig is
// loud (Apple's dispatch validator will flag the 404).

// ---------------------------------------------------------------------------
// Apple App Site Association
// ---------------------------------------------------------------------------

// aasaComponent is one entry in the AASA `components` array — an
// optional path pattern + optional excludes/caveats. Apple's docs:
// https://developer.apple.com/documentation/xcode/supporting-associated-domains
type aasaComponent struct {
	Path    string `json:"/,omitempty"`
	Exclude bool   `json:"exclude,omitempty"`
	Comment string `json:"comment,omitempty"`
}

type aasaDetails struct {
	AppIDs     []string        `json:"appIDs"`
	Components []aasaComponent `json:"components"`
}

type aasaApplinks struct {
	Apps    []string      `json:"apps"`
	Details []aasaDetails `json:"details"`
}

type aasaRoot struct {
	Applinks aasaApplinks `json:"applinks"`
}

// universalLinkComponents is the single source of truth for which URL
// paths the iOS app claims. Order matters — Apple evaluates the list
// top-to-bottom and the first match wins. Excluded paths first so
// there's no ambiguity for admin / auth routes.
var universalLinkComponents = []aasaComponent{
	{Path: "/admin/*", Exclude: true, Comment: "admin routes stay in the browser"},
	{Path: "/login*", Exclude: true, Comment: "auth flows stay in the browser"},
	{Path: "/register*", Exclude: true},
	{Path: "/reset-password*", Exclude: true},
	{Path: "/email/*", Exclude: true, Comment: "unsubscribe links must render in the browser"},
	{Path: "/.well-known/*", Exclude: true},
	{Path: "/health", Exclude: true},
	{Path: "/metrics", Exclude: true},
	// Everything user-visible the app can render.
	{Path: "/users/*/matchup/*"},
	{Path: "/brackets/*"},
	{Path: "/users/*"},
	{Path: "/privacy"},
	{Path: "/terms"},
	{Path: "/"},
}

// AppleAppSiteAssociationHandler serves the AASA JSON that Apple's
// CDN pulls when a user taps a matchup.com link. Returns 404 when
// APNS_TEAM_ID + APNS_BUNDLE_ID aren't set (dev without iOS config).
func AppleAppSiteAssociationHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID := os.Getenv("APNS_TEAM_ID")
		bundleID := os.Getenv("APNS_BUNDLE_ID")
		if teamID == "" || bundleID == "" {
			// Plain-text 404 so Apple's dispatch validator can clearly
			// report "not configured" rather than producing a misleading
			// JSON parse error.
			http.Error(w, "apple-app-site-association not configured", http.StatusNotFound)
			return
		}

		body := aasaRoot{
			Applinks: aasaApplinks{
				Apps: []string{},
				Details: []aasaDetails{
					{
						AppIDs:     []string{teamID + "." + bundleID},
						Components: universalLinkComponents,
					},
				},
			},
		}

		// Apple requires application/json even though the URL has no
		// extension. Moderate caching — 1h lets the CDN pick up env
		// changes within a working session without hammering the API.
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_ = json.NewEncoder(w).Encode(body)
	}
}

// ---------------------------------------------------------------------------
// Android App Links — assetlinks.json
// ---------------------------------------------------------------------------

type assetLinkTarget struct {
	Namespace              string   `json:"namespace"`
	PackageName            string   `json:"package_name"`
	SHA256CertFingerprints []string `json:"sha256_cert_fingerprints"`
}

type assetLink struct {
	Relation []string        `json:"relation"`
	Target   assetLinkTarget `json:"target"`
}

// AndroidAssetLinksHandler serves assetlinks.json. Unlike Apple's
// file this one doesn't carry a path list — Android reads path
// matching off the app's manifest, not the web origin — so the
// handler is simpler: one delegate_permission/common.handle_all_urls
// entry per fingerprint.
//
// ANDROID_SHA256_FINGERPRINTS can list multiple fingerprints
// comma-separated so debug / release / Play App Signing certs can
// coexist. Each fingerprint is a colon-separated hex string.
func AndroidAssetLinksHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pkg := os.Getenv("ANDROID_PACKAGE_NAME")
		rawFPs := os.Getenv("ANDROID_SHA256_FINGERPRINTS")
		if pkg == "" || rawFPs == "" {
			http.Error(w, "assetlinks not configured", http.StatusNotFound)
			return
		}

		fingerprints := parseFingerprints(rawFPs)
		if len(fingerprints) == 0 {
			http.Error(w, "assetlinks not configured", http.StatusNotFound)
			return
		}

		body := []assetLink{
			{
				Relation: []string{"delegate_permission/common.handle_all_urls"},
				Target: assetLinkTarget{
					Namespace:              "android_app",
					PackageName:            pkg,
					SHA256CertFingerprints: fingerprints,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_ = json.NewEncoder(w).Encode(body)
	}
}

// parseFingerprints splits a comma-separated env var into individual
// fingerprints, trimming whitespace. Empty slots are dropped (so a
// trailing comma or double comma in the env var is tolerated).
func parseFingerprints(raw string) []string {
	out := make([]string, 0, 4)
	for _, piece := range strings.Split(raw, ",") {
		if fp := strings.TrimSpace(piece); fp != "" {
			out = append(out, fp)
		}
	}
	return out
}
