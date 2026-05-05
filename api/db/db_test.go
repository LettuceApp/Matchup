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

func TestGenerateShortID_LengthAndAlphabet(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := GenerateShortID()
		if len(id) != 8 {
			t.Fatalf("GenerateShortID length = %d, want 8 (iter %d, got %q)", len(id), i, id)
		}
		for _, c := range id {
			inAlpha := (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
			if !inAlpha {
				t.Errorf("GenerateShortID = %q — char %q outside base62 alphabet", id, c)
			}
		}
	}
}

func TestGenerateShortID_NoCollisionsOverLargeSample(t *testing.T) {
	// We claim 62^8 ≈ 2.18e14 possibilities; 50k samples should see no
	// collisions (birthday-paradox expected first collision ≈ sqrt(N) ≈
	// 14.7M, well beyond 50k). If this test fails, crypto/rand has
	// actually broken — worth knowing loudly.
	const n = 50_000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := GenerateShortID()
		if _, dup := seen[id]; dup {
			t.Fatalf("collision at iter %d: %q (should be vanishingly unlikely)", i, id)
		}
		seen[id] = struct{}{}
	}
}

func TestIsUniqueViolation(t *testing.T) {
	if IsUniqueViolation(nil) {
		t.Error("nil must not be classified as unique violation")
	}
	// A non-pq error must return false — don't match on error strings.
	if IsUniqueViolation(errorsNew("duplicate key value")) {
		t.Error("plain errors.New should not count as unique violation")
	}
}

// errorsNew is a tiny shim so this test file doesn't need to import
// "errors" for a single New call.
func errorsNew(s string) error { return &stringErr{s} }

type stringErr struct{ s string }

func (e *stringErr) Error() string { return e.s }
