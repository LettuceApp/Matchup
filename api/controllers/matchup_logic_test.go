package controllers

import (
	"testing"

	"Matchup/models"
)

// ---- normalizeMatchupDurationSeconds ----

func TestNormalizeMatchupDurationSeconds(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{"zero becomes default", 0, 86400},
		{"negative becomes default", -5, 86400},
		{"below min becomes min", 30, 60},
		{"at min is unchanged", 60, 60},
		{"above max becomes max", 90001, 86400},
		{"at max is unchanged", 86400, 86400},
		{"valid passthrough", 3600, 3600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeMatchupDurationSeconds(tt.input)
			if got != tt.want {
				t.Errorf("normalizeMatchupDurationSeconds(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// ---- determineMatchupWinner ----

func TestDetermineMatchupWinner(t *testing.T) {
	t.Run("no items returns error", func(t *testing.T) {
		m := &models.Matchup{}
		_, err := determineMatchupWinner(m)
		if err == nil {
			t.Fatal("expected error for empty items")
		}
	})

	t.Run("explicit winner in items is returned", func(t *testing.T) {
		id := uint(42)
		m := &models.Matchup{
			WinnerItemID: &id,
			Items: []models.MatchupItem{
				{ID: 42, Votes: 3},
				{ID: 43, Votes: 5},
			},
		}
		got, err := determineMatchupWinner(m)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 42 {
			t.Errorf("got %d, want 42", got)
		}
	})

	t.Run("explicit winner not in items returns error", func(t *testing.T) {
		id := uint(99)
		m := &models.Matchup{
			WinnerItemID: &id,
			Items: []models.MatchupItem{
				{ID: 1, Votes: 3},
			},
		}
		_, err := determineMatchupWinner(m)
		if err == nil {
			t.Fatal("expected error when winner ID not found in items")
		}
	})

	t.Run("clear vote leader is returned", func(t *testing.T) {
		m := &models.Matchup{
			Items: []models.MatchupItem{
				{ID: 1, Votes: 5},
				{ID: 2, Votes: 2},
			},
		}
		got, err := determineMatchupWinner(m)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 1 {
			t.Errorf("got %d, want 1", got)
		}
	})

	t.Run("exact two-way tie returns error", func(t *testing.T) {
		m := &models.Matchup{
			Items: []models.MatchupItem{
				{ID: 1, Votes: 3},
				{ID: 2, Votes: 3},
			},
		}
		_, err := determineMatchupWinner(m)
		if err == nil {
			t.Fatal("expected error for tied matchup")
		}
	})

	t.Run("three-way tie returns error", func(t *testing.T) {
		m := &models.Matchup{
			Items: []models.MatchupItem{
				{ID: 1, Votes: 2},
				{ID: 2, Votes: 2},
				{ID: 3, Votes: 2},
			},
		}
		_, err := determineMatchupWinner(m)
		if err == nil {
			t.Fatal("expected error for three-way tie")
		}
	})
}

// ---- determineMatchupWinnerByVotes ----

func TestDetermineMatchupWinnerByVotes(t *testing.T) {
	t.Run("no items returns error", func(t *testing.T) {
		m := &models.Matchup{}
		_, err := determineMatchupWinnerByVotes(m)
		if err == nil {
			t.Fatal("expected error for empty items")
		}
	})

	t.Run("single item is returned", func(t *testing.T) {
		m := &models.Matchup{Items: []models.MatchupItem{{ID: 7, Votes: 0}}}
		got, err := determineMatchupWinnerByVotes(m)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 7 {
			t.Errorf("got %d, want 7", got)
		}
	})

	t.Run("clear winner by votes", func(t *testing.T) {
		m := &models.Matchup{
			Items: []models.MatchupItem{
				{ID: 1, Votes: 2},
				{ID: 2, Votes: 10},
				{ID: 3, Votes: 5},
			},
		}
		got, err := determineMatchupWinnerByVotes(m)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 2 {
			t.Errorf("got %d, want 2", got)
		}
	})

	t.Run("tie returns first max item encountered", func(t *testing.T) {
		m := &models.Matchup{
			Items: []models.MatchupItem{
				{ID: 1, Votes: 5},
				{ID: 2, Votes: 5},
			},
		}
		got, err := determineMatchupWinnerByVotes(m)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 1 {
			t.Errorf("got %d, want 1 (first item in tie)", got)
		}
	})
}

// ---- parseSeedValue ----

func TestParseSeedValue(t *testing.T) {
	tests := []struct {
		label     string
		wantSeed  int
		wantFound bool
	}{
		{"Seed 1", 1, true},
		{"Seed 16", 16, true},
		{"1", 1, true},
		{"42nd", 42, true},
		{"", 0, false},
		{"abc", 0, false},
		{"Seedless", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			seed, found := parseSeedValue(tt.label)
			if seed != tt.wantSeed || found != tt.wantFound {
				t.Errorf("parseSeedValue(%q) = (%d, %v), want (%d, %v)",
					tt.label, seed, found, tt.wantSeed, tt.wantFound)
			}
		})
	}
}

// ---- determineMatchupWinnerByVotesOrSeed ----

func TestDetermineMatchupWinnerByVotesOrSeed(t *testing.T) {
	t.Run("no items returns error", func(t *testing.T) {
		m := &models.Matchup{}
		_, err := determineMatchupWinnerByVotesOrSeed(m)
		if err == nil {
			t.Fatal("expected error for empty items")
		}
	})

	t.Run("clear vote leader is returned", func(t *testing.T) {
		m := &models.Matchup{
			Items: []models.MatchupItem{
				{ID: 1, Item: "Seed 3", Votes: 10},
				{ID: 2, Item: "Seed 1", Votes: 2},
			},
		}
		got, err := determineMatchupWinnerByVotesOrSeed(m)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 1 {
			t.Errorf("got %d, want 1", got)
		}
	})

	t.Run("tie resolved by lower seed number", func(t *testing.T) {
		m := &models.Matchup{
			Items: []models.MatchupItem{
				{ID: 10, Item: "Seed 4", Votes: 5},
				{ID: 20, Item: "Seed 1", Votes: 5},
			},
		}
		got, err := determineMatchupWinnerByVotesOrSeed(m)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 20 {
			t.Errorf("got %d, want 20 (Seed 1 should win tiebreak)", got)
		}
	})

	t.Run("tie with no seed labels returns error", func(t *testing.T) {
		m := &models.Matchup{
			Items: []models.MatchupItem{
				{ID: 1, Item: "Alpha", Votes: 5},
				{ID: 2, Item: "Beta", Votes: 5},
			},
		}
		_, err := determineMatchupWinnerByVotesOrSeed(m)
		if err == nil {
			t.Fatal("expected error when tied and no seeds available")
		}
	})

	t.Run("tie with one seeded item uses seeded item", func(t *testing.T) {
		m := &models.Matchup{
			Items: []models.MatchupItem{
				{ID: 1, Item: "Seed 2", Votes: 5},
				{ID: 2, Item: "Challenger", Votes: 5},
			},
		}
		got, err := determineMatchupWinnerByVotesOrSeed(m)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 1 {
			t.Errorf("got %d, want 1 (only seeded item)", got)
		}
	})
}
