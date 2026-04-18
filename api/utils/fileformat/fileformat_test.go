package fileformat

import (
	"strings"
	"testing"
)

func TestUniqueFormat_PreservesExtension(t *testing.T) {
	tests := []struct {
		name  string
		input string
		ext   string
	}{
		{"jpg", "test.jpg", ".jpg"},
		{"png", "photo.png", ".png"},
		{"webp", "image.webp", ".webp"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := UniqueFormat(tc.input)
			if !strings.HasSuffix(result, tc.ext) {
				t.Errorf("UniqueFormat(%q) = %q, want suffix %q", tc.input, result, tc.ext)
			}
		})
	}
}

func TestUniqueFormat_ProducesUniqueNames(t *testing.T) {
	a := UniqueFormat("photo.jpg")
	b := UniqueFormat("photo.jpg")
	if a == b {
		t.Errorf("UniqueFormat produced identical names on two calls: %q", a)
	}
}

func TestUniqueFormat_NoExtension(t *testing.T) {
	result := UniqueFormat("README")
	if !strings.HasPrefix(result, "README-") {
		t.Errorf("UniqueFormat(%q) = %q, want prefix %q", "README", result, "README-")
	}
	// Should not contain a dot since the original has no extension.
	if strings.Contains(result, ".") {
		t.Errorf("UniqueFormat(%q) = %q, unexpected dot in result for extensionless input", "README", result)
	}
}
