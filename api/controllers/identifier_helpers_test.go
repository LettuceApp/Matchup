package controllers

import "testing"

func TestIsUUIDLike(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid lowercase UUID", "550e8400-e29b-41d4-a716-446655440000", true},
		{"valid uppercase UUID", "550E8400-E29B-41D4-A716-446655440000", true},
		{"short string", "abc", false},
		{"36 chars wrong dash positions", "550e8400e29b-41d4-a716-4466554400-0", false},
		{"empty string", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isUUIDLike(tt.input)
			if got != tt.want {
				t.Errorf("isUUIDLike(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
