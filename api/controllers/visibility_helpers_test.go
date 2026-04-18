package controllers

import (
	"strings"
	"testing"
)

func TestNormalizeVisibility(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"mutuals stays mutuals", "mutuals", "mutuals"},
		{"public stays public", "public", "public"},
		{"unknown defaults to public", "anything", "public"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeVisibility(tt.input)
			if got != tt.want {
				t.Errorf("normalizeVisibility(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestVisibilityErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		reason   string
		contains string
	}{
		{"mutuals reason mentions mutual", "mutuals", "mutual"},
		{"followers reason mentions followers", "followers", "followers"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := visibilityErrorMessage(tt.reason)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("visibilityErrorMessage(%q) = %q, want it to contain %q", tt.reason, got, tt.contains)
			}
		})
	}
}
