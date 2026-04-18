package security

import (
	"testing"
)

func TestTokenHash(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "non-empty for regular input", input: "hello"},
		{name: "non-empty for empty input", input: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TokenHash(tt.input)
			if result == "" {
				t.Errorf("TokenHash(%q) returned empty string", tt.input)
			}
		})
	}
}

func TestTokenHashSameInputDifferentResults(t *testing.T) {
	input := "duplicate-test"
	result1 := TokenHash(input)
	result2 := TokenHash(input)

	if result1 == result2 {
		t.Errorf("TokenHash(%q) produced identical results on two calls: %q", input, result1)
	}
}

func TestTokenHashDifferentInputsDifferentResults(t *testing.T) {
	result1 := TokenHash("input-alpha")
	result2 := TokenHash("input-bravo")

	if result1 == result2 {
		t.Errorf("TokenHash produced identical results for different inputs: %q", result1)
	}
}
