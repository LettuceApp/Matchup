package controllers

import (
	"encoding/base64"
	"testing"
	"time"
)

// ---- parseLimit ----

func TestParseLimit(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"valid number", "10", 10},
		{"zero defaults", "0", 20},
		{"negative defaults", "-1", 20},
		{"above max defaults", "200", 20},
		{"non-numeric defaults", "abc", 20},
		{"empty defaults", "", 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLimit(tt.input)
			if got != tt.want {
				t.Errorf("parseLimit(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// ---- followCursor encode/decode roundtrip ----

func TestFollowCursorRoundtrip(t *testing.T) {
	t.Run("encode then decode returns same values", func(t *testing.T) {
		original := followCursor{
			ID:        42,
			CreatedAt: time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
		}
		encoded, err := encodeFollowCursor(original)
		if err != nil {
			t.Fatalf("encodeFollowCursor: unexpected error: %v", err)
		}
		decoded, err := parseFollowCursor(encoded)
		if err != nil {
			t.Fatalf("parseFollowCursor: unexpected error: %v", err)
		}
		if decoded.ID != original.ID {
			t.Errorf("ID = %d, want %d", decoded.ID, original.ID)
		}
		if !decoded.CreatedAt.Equal(original.CreatedAt) {
			t.Errorf("CreatedAt = %v, want %v", decoded.CreatedAt, original.CreatedAt)
		}
	})
}

func TestParseFollowCursorErrors(t *testing.T) {
	t.Run("invalid base64 returns error", func(t *testing.T) {
		_, err := parseFollowCursor("not-valid-base64!!!")
		if err == nil {
			t.Errorf("parseFollowCursor with invalid base64: want error, got nil")
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		encoded := base64.StdEncoding.EncodeToString([]byte("{invalid json"))
		_, err := parseFollowCursor(encoded)
		if err == nil {
			t.Errorf("parseFollowCursor with invalid JSON: want error, got nil")
		}
	})
}

// ---- buildFollowListResponse ----

func TestBuildFollowListResponse(t *testing.T) {
	t.Setenv("S3_BUCKET", "test-bucket")
	t.Setenv("AWS_REGION", "us-east-2")

	makeRows := func(n int) []followRow {
		rows := make([]followRow, n)
		for i := 0; i < n; i++ {
			rows[i] = followRow{
				FollowID:        uint(i + 1),
				FollowCreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Hour),
				UserID:          uint(100 + i),
				PublicID:        "pub-id",
				Username:        "user",
				Email:           "u@example.com",
				AvatarPath:      "",
				IsAdmin:         false,
				FollowersCount:  0,
				FollowingCount:  0,
			}
		}
		return rows
	}

	t.Run("truncates when rows exceed limit and returns cursor", func(t *testing.T) {
		rows := makeRows(6)
		limit := 5
		dtos, cursor := buildFollowListResponse(rows, limit, false, map[uint]bool{}, map[uint]bool{})
		if len(dtos) != limit {
			t.Errorf("len(dtos) = %d, want %d", len(dtos), limit)
		}
		if cursor == nil {
			t.Errorf("cursor is nil, want non-nil when rows > limit")
		}
	})

	t.Run("no truncation when rows within limit and no cursor", func(t *testing.T) {
		rows := makeRows(3)
		limit := 5
		dtos, cursor := buildFollowListResponse(rows, limit, false, map[uint]bool{}, map[uint]bool{})
		if len(dtos) != 3 {
			t.Errorf("len(dtos) = %d, want %d", len(dtos), 3)
		}
		if cursor != nil {
			t.Errorf("cursor = %q, want nil when rows <= limit", *cursor)
		}
	})
}
