package controllers

import (
	"testing"
	"time"

	"Matchup/models"
)

// ---- formatSeedLabel ----

func TestFormatSeedLabel(t *testing.T) {
	tests := []struct {
		name string
		seed int
		label string
		want string
	}{
		{"basic label", 1, "Alpha", "Seed 1 - Alpha"},
		{"empty name omits dash", 1, "", "Seed 1"},
		{"strips old seed prefix", 3, "Seed 2 - Beta", "Seed 3 - Beta"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSeedLabel(tt.seed, tt.label)
			if got != tt.want {
				t.Errorf("formatSeedLabel(%d, %q) = %q, want %q", tt.seed, tt.label, got, tt.want)
			}
		})
	}
}

// ---- stripSeedPrefix ----

func TestStripSeedPrefix(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"removes seed prefix", "Seed 1 - Alpha", "Alpha"},
		{"no prefix unchanged", "Alpha", "Alpha"},
		{"empty string", "", ""},
		{"seed number only no name", "Seed 5", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripSeedPrefix(tt.input)
			if got != tt.want {
				t.Errorf("stripSeedPrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---- groupMatchupsByRound ----

func TestGroupMatchupsByRound(t *testing.T) {
	t.Run("empty slice returns empty map", func(t *testing.T) {
		got := groupMatchupsByRound(nil)
		if len(got) != 0 {
			t.Errorf("groupMatchupsByRound(nil) returned %d rounds, want 0", len(got))
		}
	})

	t.Run("single matchup in round 1", func(t *testing.T) {
		r := 1
		matchups := []models.Matchup{{Round: &r}}
		got := groupMatchupsByRound(matchups)
		if len(got[1]) != 1 {
			t.Errorf("round 1 count = %d, want 1", len(got[1]))
		}
	})

	t.Run("multiple rounds", func(t *testing.T) {
		r1, r2 := 1, 2
		matchups := []models.Matchup{
			{Round: &r1},
			{Round: &r1},
			{Round: &r2},
		}
		got := groupMatchupsByRound(matchups)
		if len(got[1]) != 2 {
			t.Errorf("round 1 count = %d, want 2", len(got[1]))
		}
		if len(got[2]) != 1 {
			t.Errorf("round 2 count = %d, want 1", len(got[2]))
		}
	})

	t.Run("skips matchups with nil Round", func(t *testing.T) {
		r1 := 1
		matchups := []models.Matchup{
			{Round: &r1},
			{Round: nil},
		}
		got := groupMatchupsByRound(matchups)
		total := 0
		for _, v := range got {
			total += len(v)
		}
		if total != 1 {
			t.Errorf("total grouped matchups = %d, want 1", total)
		}
	})
}

// ---- setBracketRoundWindow ----

func TestSetBracketRoundWindow(t *testing.T) {
	t.Run("timer mode sets times", func(t *testing.T) {
		b := &models.Bracket{
			AdvanceMode:          "timer",
			RoundDurationSeconds: 3600,
		}
		now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
		setBracketRoundWindow(b, now)
		if b.RoundStartedAt == nil {
			t.Fatal("RoundStartedAt is nil, want non-nil")
		}
		if !b.RoundStartedAt.Equal(now) {
			t.Errorf("RoundStartedAt = %v, want %v", *b.RoundStartedAt, now)
		}
		if b.RoundEndsAt == nil {
			t.Fatal("RoundEndsAt is nil, want non-nil")
		}
		wantEnd := now.Add(3600 * time.Second)
		if !b.RoundEndsAt.Equal(wantEnd) {
			t.Errorf("RoundEndsAt = %v, want %v", *b.RoundEndsAt, wantEnd)
		}
	})

	t.Run("manual mode clears times", func(t *testing.T) {
		now := time.Now()
		startedAt := now
		endsAt := now.Add(time.Hour)
		b := &models.Bracket{
			AdvanceMode:    "manual",
			RoundStartedAt: &startedAt,
			RoundEndsAt:    &endsAt,
		}
		setBracketRoundWindow(b, now)
		if b.RoundStartedAt != nil {
			t.Errorf("RoundStartedAt = %v, want nil", *b.RoundStartedAt)
		}
		if b.RoundEndsAt != nil {
			t.Errorf("RoundEndsAt = %v, want nil", *b.RoundEndsAt)
		}
	})

	t.Run("timer with negative duration uses default", func(t *testing.T) {
		b := &models.Bracket{
			AdvanceMode:          "timer",
			RoundDurationSeconds: -10,
		}
		now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
		setBracketRoundWindow(b, now)
		if b.RoundDurationSeconds != defaultRoundDurationSeconds {
			t.Errorf("RoundDurationSeconds = %d, want %d", b.RoundDurationSeconds, defaultRoundDurationSeconds)
		}
		wantEnd := now.Add(time.Duration(defaultRoundDurationSeconds) * time.Second)
		if b.RoundEndsAt == nil || !b.RoundEndsAt.Equal(wantEnd) {
			t.Errorf("RoundEndsAt = %v, want %v", b.RoundEndsAt, wantEnd)
		}
	})
}

// ---- ptrUint / ptrInt ----

func TestPtrUint(t *testing.T) {
	t.Run("returns pointer to value", func(t *testing.T) {
		v := uint(42)
		got := ptrUint(v)
		if got == nil || *got != v {
			t.Errorf("ptrUint(%d) = %v, want pointer to %d", v, got, v)
		}
	})
}

func TestPtrInt(t *testing.T) {
	t.Run("returns pointer to value", func(t *testing.T) {
		v := 99
		got := ptrInt(v)
		if got == nil || *got != v {
			t.Errorf("ptrInt(%d) = %v, want pointer to %d", v, got, v)
		}
	})
}
