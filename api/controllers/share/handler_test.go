package share

import "testing"

func TestValidShortID(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		// Representative valid outputs from GenerateShortID.
		{"all lowercase", "abcdefgh", true},
		{"all uppercase", "ABCDEFGH", true},
		{"all digits", "01234567", true},
		{"mixed", "Kj3mN9xP", true},

		// Length policing — share URLs come from user input and we
		// shouldn't pass garbage to the SQL layer.
		{"too short", "abc123", false},
		{"too long", "abcdefghij", false},
		{"empty", "", false},

		// Disallowed character classes.
		{"dash", "abcd-123", false},
		{"dot", "abcd.123", false},
		{"slash", "abcd/123", false},
		{"unicode", "abcd1café", false},
		{"sql apostrophe", "a'bcdef1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validShortID(tc.input); got != tc.want {
				t.Errorf("validShortID(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
