package controllers

import "testing"

func TestBuildVariantKey(t *testing.T) {
	tests := []struct {
		name    string
		keyBase string
		suffix  string
		want    string
	}{
		{
			name:    "with extension",
			keyBase: "dir/file.png",
			suffix:  "_thumb",
			want:    "dir/file_thumb.jpg",
		},
		{
			name:    "without extension",
			keyBase: "dir/file",
			suffix:  "_thumb",
			want:    "dir/file_thumb.jpg",
		},
		{
			name:    "empty suffix for full size",
			keyBase: "dir/file.png",
			suffix:  "",
			want:    "dir/file.jpg",
		},
		{
			name:    "dot in directory name",
			keyBase: "some.dir/file.png",
			suffix:  "_medium",
			want:    "some.dir/file_medium.jpg",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildVariantKey(tt.keyBase, tt.suffix)
			if got != tt.want {
				t.Errorf("buildVariantKey(%q, %q) = %q, want %q", tt.keyBase, tt.suffix, got, tt.want)
			}
		})
	}
}
