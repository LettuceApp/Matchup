package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Well-known handler tests — pure httptest + Setenv, no DB, no build
// tag. Fast; runs in every `go test ./...` invocation.

func TestAASA_HappyPath(t *testing.T) {
	t.Setenv("APNS_TEAM_ID", "ABCDE12345")
	t.Setenv("APNS_BUNDLE_ID", "com.matchup.app")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/apple-app-site-association", nil)
	w := httptest.NewRecorder()
	AppleAppSiteAssociationHandler()(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", got)
	}

	var parsed aasaRoot
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if len(parsed.Applinks.Details) != 1 {
		t.Fatalf("expected 1 details entry, got %d", len(parsed.Applinks.Details))
	}
	d := parsed.Applinks.Details[0]
	if want := "ABCDE12345.com.matchup.app"; len(d.AppIDs) != 1 || d.AppIDs[0] != want {
		t.Errorf("expected appIDs=[%q], got %v", want, d.AppIDs)
	}
	if len(d.Components) == 0 {
		t.Error("expected non-empty components list")
	}
}

// The components list is load-bearing — anything added or removed
// changes which links the iOS app intercepts. This test pins the
// exact list so reviewers have to see the diff when it shifts.
func TestAASA_ClaimsExpectedPaths(t *testing.T) {
	t.Setenv("APNS_TEAM_ID", "ABCDE12345")
	t.Setenv("APNS_BUNDLE_ID", "com.matchup.app")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/apple-app-site-association", nil)
	w := httptest.NewRecorder()
	AppleAppSiteAssociationHandler()(w, req)

	var parsed aasaRoot
	_ = json.Unmarshal(w.Body.Bytes(), &parsed)

	// Pinned expected paths — if you add a new claim in
	// universalLinkComponents, update this list too.
	wantPaths := []struct {
		path    string
		exclude bool
	}{
		{"/admin/*", true},
		{"/login*", true},
		{"/register*", true},
		{"/reset-password*", true},
		{"/email/*", true},
		{"/.well-known/*", true},
		{"/health", true},
		{"/metrics", true},
		{"/users/*/matchup/*", false},
		{"/brackets/*", false},
		{"/users/*", false},
		{"/privacy", false},
		{"/terms", false},
		{"/", false},
	}
	got := parsed.Applinks.Details[0].Components
	if len(got) != len(wantPaths) {
		t.Fatalf("expected %d components, got %d", len(wantPaths), len(got))
	}
	for i, want := range wantPaths {
		if got[i].Path != want.path {
			t.Errorf("components[%d].path = %q, want %q", i, got[i].Path, want.path)
		}
		if got[i].Exclude != want.exclude {
			t.Errorf("components[%d].exclude = %v, want %v", i, got[i].Exclude, want.exclude)
		}
	}
}

func TestAASA_MissingEnvReturns404(t *testing.T) {
	// Explicit empty to override any dev .env leakage into the test
	// process — t.Setenv restores the previous value at test end.
	t.Setenv("APNS_TEAM_ID", "")
	t.Setenv("APNS_BUNDLE_ID", "")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/apple-app-site-association", nil)
	w := httptest.NewRecorder()
	AppleAppSiteAssociationHandler()(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 with missing env, got %d", w.Code)
	}
}

func TestAssetLinks_HappyPath(t *testing.T) {
	t.Setenv("ANDROID_PACKAGE_NAME", "com.matchup.app")
	t.Setenv("ANDROID_SHA256_FINGERPRINTS", "AA:BB:CC,DD:EE:FF")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/assetlinks.json", nil)
	w := httptest.NewRecorder()
	AndroidAssetLinksHandler()(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", got)
	}

	var parsed []assetLink
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 assetlink, got %d", len(parsed))
	}
	e := parsed[0]
	if e.Target.PackageName != "com.matchup.app" {
		t.Errorf("package_name = %q", e.Target.PackageName)
	}
	if got, want := e.Target.SHA256CertFingerprints, []string{"AA:BB:CC", "DD:EE:FF"}; !equalStrings(got, want) {
		t.Errorf("fingerprints = %v, want %v", got, want)
	}
	if len(e.Relation) != 1 || e.Relation[0] != "delegate_permission/common.handle_all_urls" {
		t.Errorf("relation = %v", e.Relation)
	}
}

// Multi-fingerprint parsing edge cases — trailing commas, whitespace,
// double commas. The parser should tolerate all of these so ops can
// edit the env var in a copy/paste-friendly way.
func TestAssetLinks_TolerantFingerprintParsing(t *testing.T) {
	t.Setenv("ANDROID_PACKAGE_NAME", "com.matchup.app")
	t.Setenv("ANDROID_SHA256_FINGERPRINTS", " AA:BB:CC ,, DD:EE:FF , ")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/assetlinks.json", nil)
	w := httptest.NewRecorder()
	AndroidAssetLinksHandler()(w, req)

	var parsed []assetLink
	_ = json.Unmarshal(w.Body.Bytes(), &parsed)

	want := []string{"AA:BB:CC", "DD:EE:FF"}
	if got := parsed[0].Target.SHA256CertFingerprints; !equalStrings(got, want) {
		t.Errorf("fingerprints = %v, want %v", got, want)
	}
}

func TestAssetLinks_MissingEnvReturns404(t *testing.T) {
	t.Setenv("ANDROID_PACKAGE_NAME", "")
	t.Setenv("ANDROID_SHA256_FINGERPRINTS", "")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/assetlinks.json", nil)
	w := httptest.NewRecorder()
	AndroidAssetLinksHandler()(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 with missing env, got %d", w.Code)
	}
}

// Empty fingerprint list (package set but fingerprints absent) still
// returns 404 — a manifest with zero fingerprints would never verify
// so there's no point serving it.
func TestAssetLinks_EmptyFingerprintsReturn404(t *testing.T) {
	t.Setenv("ANDROID_PACKAGE_NAME", "com.matchup.app")
	t.Setenv("ANDROID_SHA256_FINGERPRINTS", " , , ")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/assetlinks.json", nil)
	w := httptest.NewRecorder()
	AndroidAssetLinksHandler()(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 when all fingerprints are empty, got %d", w.Code)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
