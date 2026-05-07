package models

import (
	"testing"
	"time"
)

// Prepare() used to call html.EscapeString on the title before insert,
// but React auto-escapes JSX text nodes at render — the backend escape
// was double-encoding and surfacing as &#39; etc. in the UI. The test
// now asserts the new contract: titles are trimmed but stored raw.
func TestBracketPrepare_TrimsTitle(t *testing.T) {
	b := Bracket{Title: "  <b>Title</b>  "}
	b.Prepare()
	want := "<b>Title</b>"
	if b.Title != want {
		t.Errorf("Title = %q, want %q", b.Title, want)
	}
}

func TestBracketPrepare_PreservesApostrophe(t *testing.T) {
	b := Bracket{Title: "Greatest '90s Movie"}
	b.Prepare()
	want := "Greatest '90s Movie"
	if b.Title != want {
		t.Errorf("Title = %q, want %q (apostrophe must round-trip un-encoded)",
			b.Title, want)
	}
}

func TestBracketPrepare_DefaultsStatus(t *testing.T) {
	b := Bracket{}
	b.Prepare()
	if b.Status != "draft" {
		t.Errorf("Status = %q, want %q", b.Status, "draft")
	}
}

func TestBracketPrepare_DefaultsCurrentRound(t *testing.T) {
	b := Bracket{}
	b.Prepare()
	if b.CurrentRound != 1 {
		t.Errorf("CurrentRound = %d, want 1", b.CurrentRound)
	}
}

func TestBracketPrepare_DefaultsAdvanceMode(t *testing.T) {
	b := Bracket{}
	b.Prepare()
	if b.AdvanceMode != "manual" {
		t.Errorf("AdvanceMode = %q, want %q", b.AdvanceMode, "manual")
	}
}

func TestBracketPrepare_ClampsNegativeDuration(t *testing.T) {
	b := Bracket{RoundDurationSeconds: -100}
	b.Prepare()
	if b.RoundDurationSeconds != 0 {
		t.Errorf("RoundDurationSeconds = %d, want 0", b.RoundDurationSeconds)
	}
}

func TestBracketPrepare_DefaultsVisibility(t *testing.T) {
	b := Bracket{}
	b.Prepare()
	if b.Visibility != "public" {
		t.Errorf("Visibility = %q, want %q", b.Visibility, "public")
	}
}

func TestBracketPrepare_SetsTimestamps(t *testing.T) {
	before := time.Now().Add(-time.Second)
	b := Bracket{}
	b.Prepare()
	after := time.Now().Add(time.Second)

	if b.CreatedAt.Before(before) || b.CreatedAt.After(after) {
		t.Errorf("CreatedAt = %v, want between %v and %v", b.CreatedAt, before, after)
	}
	if b.UpdatedAt.Before(before) || b.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt = %v, want between %v and %v", b.UpdatedAt, before, after)
	}
}

func TestBracketValidate(t *testing.T) {
	tests := []struct {
		name    string
		bracket Bracket
		wantKey string
	}{
		{
			name:    "missing title",
			bracket: Bracket{AuthorID: 1, Status: "draft", AdvanceMode: "manual"},
			wantKey: "Required_title",
		},
		{
			name:    "missing author",
			bracket: Bracket{Title: "Test", Status: "draft", AdvanceMode: "manual"},
			wantKey: "Required_author",
		},
		{
			name:    "invalid advance mode",
			bracket: Bracket{Title: "Test", AuthorID: 1, Status: "draft", AdvanceMode: "auto"},
			wantKey: "advance_mode",
		},
		{
			name:    "timer duration too low",
			bracket: Bracket{Title: "Test", AuthorID: 1, Status: "draft", AdvanceMode: "timer", RoundDurationSeconds: 10},
			wantKey: "round_duration_seconds",
		},
		{
			name:    "timer duration too high",
			bracket: Bracket{Title: "Test", AuthorID: 1, Status: "draft", AdvanceMode: "timer", RoundDurationSeconds: 100000},
			wantKey: "round_duration_seconds",
		},
		{
			name:    "valid bracket",
			bracket: Bracket{Title: "Test", AuthorID: 1, Status: "draft", AdvanceMode: "manual"},
			wantKey: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := tc.bracket.Validate()
			if tc.wantKey == "" {
				if len(errs) != 0 {
					t.Errorf("expected no errors, got %v", errs)
				}
				return
			}
			if _, ok := errs[tc.wantKey]; !ok {
				t.Errorf("expected error key %q, got %v", tc.wantKey, errs)
			}
		})
	}
}
