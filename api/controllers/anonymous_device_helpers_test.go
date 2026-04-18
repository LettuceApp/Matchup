package controllers

import "testing"

func TestHashFingerprint(t *testing.T) {
	t.Run("empty string returns empty", func(t *testing.T) {
		got := hashFingerprint("")
		if got != "" {
			t.Errorf("hashFingerprint(%q) = %q, want %q", "", got, "")
		}
	})

	t.Run("whitespace only returns empty", func(t *testing.T) {
		got := hashFingerprint("   ")
		if got != "" {
			t.Errorf("hashFingerprint(%q) = %q, want %q", "   ", got, "")
		}
	})

	t.Run("non-empty input returns non-empty hash", func(t *testing.T) {
		t.Setenv("ANON_DEVICE_SALT", "test-salt")
		got := hashFingerprint("test")
		if got == "" {
			t.Errorf("hashFingerprint(%q) returned empty, want non-empty", "test")
		}
	})

	t.Run("same input produces same output", func(t *testing.T) {
		t.Setenv("ANON_DEVICE_SALT", "test-salt")
		a := hashFingerprint("test")
		b := hashFingerprint("test")
		if a != b {
			t.Errorf("hashFingerprint determinism: got %q and %q, want equal", a, b)
		}
	})
}

func TestIsLikelyUUID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"36-char UUID returns true", "550e8400-e29b-41d4-a716-446655440000", true},
		{"short string returns false", "abc", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isLikelyUUID(tt.input)
			if got != tt.want {
				t.Errorf("isLikelyUUID(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
