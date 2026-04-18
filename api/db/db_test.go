package db

import (
	"strings"
	"testing"
)

func TestGeneratePublicID_Format(t *testing.T) {
	id := GeneratePublicID()
	if len(id) != 36 {
		t.Errorf("GeneratePublicID() length = %d, want 36", len(id))
	}
	dashPositions := []int{8, 13, 18, 23}
	for _, pos := range dashPositions {
		if id[pos] != '-' {
			t.Errorf("GeneratePublicID() char at %d = %q, want '-'", pos, string(id[pos]))
		}
	}
}

func TestGeneratePublicID_Unique(t *testing.T) {
	a := GeneratePublicID()
	b := GeneratePublicID()
	if a == b {
		t.Errorf("GeneratePublicID produced identical values: %q", a)
	}
}

func TestProcessAvatarPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "empty string",
			path: "",
			want: "",
		},
		{
			name: "full URL passthrough",
			path: "https://example.com/avatar.jpg",
			want: "https://example.com/avatar.jpg",
		},
		{
			name: "relative path to S3 URL",
			path: "avatar-abc.jpg",
			want: "https://test-bucket.s3.us-east-2.amazonaws.com/UserProfilePics/avatar-abc.jpg",
		},
		{
			name: "already prefixed not doubled",
			path: "UserProfilePics/foo.jpg",
			want: "https://test-bucket.s3.us-east-2.amazonaws.com/UserProfilePics/foo.jpg",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("S3_BUCKET", "test-bucket")
			t.Setenv("AWS_REGION", "us-east-2")

			got := ProcessAvatarPath(tc.path)
			if got != tc.want {
				t.Errorf("ProcessAvatarPath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestProcessAvatarPathSized(t *testing.T) {
	tests := []struct {
		name string
		path string
		size string
		want string
	}{
		{
			name: "thumb inserts _thumb",
			path: "avatar-abc.jpg",
			size: "thumb",
			want: "https://test-bucket.s3.us-east-2.amazonaws.com/UserProfilePics/avatar-abc_thumb.jpg",
		},
		{
			name: "medium inserts _medium",
			path: "avatar-abc.jpg",
			size: "medium",
			want: "https://test-bucket.s3.us-east-2.amazonaws.com/UserProfilePics/avatar-abc_medium.jpg",
		},
		{
			name: "empty size returns full URL",
			path: "avatar-abc.jpg",
			size: "",
			want: "https://test-bucket.s3.us-east-2.amazonaws.com/UserProfilePics/avatar-abc.jpg",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("S3_BUCKET", "test-bucket")
			t.Setenv("AWS_REGION", "us-east-2")

			got := ProcessAvatarPathSized(tc.path, tc.size)
			if got != tc.want {
				t.Errorf("ProcessAvatarPathSized(%q, %q) = %q, want %q", tc.path, tc.size, got, tc.want)
			}
		})
	}
}

func TestProcessMatchupImagePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "empty string",
			path: "",
			want: "",
		},
		{
			name: "relative path to S3 URL",
			path: "match-img.png",
			want: "https://test-bucket.s3.us-east-2.amazonaws.com/MatchupImages/match-img.png",
		},
		{
			name: "full URL passthrough",
			path: "https://cdn.example.com/img.png",
			want: "https://cdn.example.com/img.png",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("S3_BUCKET", "test-bucket")
			t.Setenv("AWS_REGION", "us-east-2")

			got := ProcessMatchupImagePath(tc.path)
			if got != tc.want {
				t.Errorf("ProcessMatchupImagePath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestProcessMatchupImagePathSized(t *testing.T) {
	tests := []struct {
		name string
		path string
		size string
		want string
	}{
		{
			name: "thumb inserts _thumb",
			path: "match-img.png",
			size: "thumb",
			want: "https://test-bucket.s3.us-east-2.amazonaws.com/MatchupImages/match-img_thumb.png",
		},
		{
			name: "already prefixed not doubled",
			path: "MatchupImages/img.jpg",
			size: "thumb",
			want: "https://test-bucket.s3.us-east-2.amazonaws.com/MatchupImages/img_thumb.jpg",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("S3_BUCKET", "test-bucket")
			t.Setenv("AWS_REGION", "us-east-2")

			got := ProcessMatchupImagePathSized(tc.path, tc.size)
			if got != tc.want {
				t.Errorf("ProcessMatchupImagePathSized(%q, %q) = %q, want %q", tc.path, tc.size, got, tc.want)
			}
		})
	}
}

func TestInsertSizeSuffix_ViaProcessFunctions(t *testing.T) {
	// insertSizeSuffix is unexported; we verify its behaviour through the
	// exported sized functions, which delegate to it.
	t.Run("suffix before extension", func(t *testing.T) {
		t.Setenv("S3_BUCKET", "test-bucket")
		t.Setenv("AWS_REGION", "us-east-2")

		got := ProcessAvatarPathSized("pic.jpeg", "thumb")
		if !strings.Contains(got, "pic_thumb.jpeg") {
			t.Errorf("expected _thumb before .jpeg, got %q", got)
		}
	})

	t.Run("no extension appends suffix", func(t *testing.T) {
		t.Setenv("S3_BUCKET", "test-bucket")
		t.Setenv("AWS_REGION", "us-east-2")

		got := ProcessAvatarPathSized("avatar", "thumb")
		if !strings.HasSuffix(got, "avatar_thumb") {
			t.Errorf("expected suffix appended for extensionless file, got %q", got)
		}
	})
}
