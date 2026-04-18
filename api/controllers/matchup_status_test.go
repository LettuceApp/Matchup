package controllers

import "testing"

func TestIsMatchupOpenStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{"active is open", "active", true},
		{"published is open", "published", true},
		{"draft is not open", "draft", false},
		{"completed is not open", "completed", false},
		{"empty is not open", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isMatchupOpenStatus(tt.status)
			if got != tt.want {
				t.Errorf("isMatchupOpenStatus(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
