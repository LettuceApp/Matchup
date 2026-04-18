package controllers

import "testing"

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty string",
			input: "",
			want:  []string{},
		},
		{
			name:  "single value",
			input: "a",
			want:  []string{"a"},
		},
		{
			name:  "multiple values with spaces",
			input: "a, b, c",
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "trims whitespace and drops empty parts",
			input: "  a , , b  ",
			want:  []string{"a", "b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitCSV(tt.input)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("splitCSV(%q) returned %d elements %v, want %d elements %v",
					tt.input, len(got), got, len(tt.want), tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitCSV(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}
