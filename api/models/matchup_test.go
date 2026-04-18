package models

import (
	"testing"
	"time"
)

func TestMatchupPrepare_SanitizesTitle(t *testing.T) {
	m := Matchup{Title: "  <b>Test</b>  "}
	m.Prepare()
	want := "&lt;b&gt;Test&lt;/b&gt;"
	if m.Title != want {
		t.Errorf("Title = %q, want %q", m.Title, want)
	}
}

func TestMatchupPrepare_DefaultsEndMode(t *testing.T) {
	m := Matchup{}
	m.Prepare()
	if m.EndMode != "manual" {
		t.Errorf("EndMode = %q, want %q", m.EndMode, "manual")
	}
}

func TestMatchupPrepare_DefaultsVisibility(t *testing.T) {
	m := Matchup{}
	m.Prepare()
	if m.Visibility != "public" {
		t.Errorf("Visibility = %q, want %q", m.Visibility, "public")
	}
}

func TestMatchupPrepare_SetsTimestamps(t *testing.T) {
	before := time.Now().Add(-time.Second)
	m := Matchup{}
	m.Prepare()
	after := time.Now().Add(time.Second)

	if m.CreatedAt.Before(before) || m.CreatedAt.After(after) {
		t.Errorf("CreatedAt = %v, want between %v and %v", m.CreatedAt, before, after)
	}
	if m.UpdatedAt.Before(before) || m.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt = %v, want between %v and %v", m.UpdatedAt, before, after)
	}
}

func TestMatchupValidate(t *testing.T) {
	tests := []struct {
		name    string
		matchup Matchup
		wantKey string
	}{
		{
			name:    "missing title",
			matchup: Matchup{AuthorID: 1},
			wantKey: "Required_title",
		},
		{
			name:    "missing author",
			matchup: Matchup{Title: "Test"},
			wantKey: "Required_author",
		},
		{
			name:    "valid matchup",
			matchup: Matchup{Title: "Test", AuthorID: 1},
			wantKey: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := tc.matchup.Validate()
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

func TestMatchupWinnerItem_ExplicitWinner(t *testing.T) {
	winnerID := uint(2)
	m := Matchup{
		WinnerItemID: &winnerID,
		Items: []MatchupItem{
			{ID: 1, Item: "A", Votes: 5},
			{ID: 2, Item: "B", Votes: 3},
		},
	}
	winner, err := m.WinnerItem()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if winner.ID != 2 {
		t.Errorf("winner ID = %d, want 2", winner.ID)
	}
}

func TestMatchupWinnerItem_ByVotes(t *testing.T) {
	m := Matchup{
		Items: []MatchupItem{
			{ID: 1, Item: "A", Votes: 10},
			{ID: 2, Item: "B", Votes: 3},
		},
	}
	winner, err := m.WinnerItem()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if winner.ID != 1 {
		t.Errorf("winner ID = %d, want 1", winner.ID)
	}
}

func TestMatchupWinnerItem_TiedReturnsError(t *testing.T) {
	m := Matchup{
		Items: []MatchupItem{
			{ID: 1, Item: "A", Votes: 5},
			{ID: 2, Item: "B", Votes: 5},
		},
	}
	_, err := m.WinnerItem()
	if err == nil {
		t.Fatal("expected error for tied matchup, got nil")
	}
}

func TestMatchupHasTie_True(t *testing.T) {
	m := Matchup{
		Items: []MatchupItem{
			{ID: 1, Votes: 5},
			{ID: 2, Votes: 5},
		},
	}
	if !m.HasTie() {
		t.Error("HasTie() = false, want true")
	}
}

func TestMatchupHasTie_False(t *testing.T) {
	m := Matchup{
		Items: []MatchupItem{
			{ID: 1, Votes: 5},
			{ID: 2, Votes: 3},
		},
	}
	if m.HasTie() {
		t.Error("HasTie() = true, want false")
	}
}
