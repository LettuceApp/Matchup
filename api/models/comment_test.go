package models

import (
	"testing"
	"time"
)

func TestCommentPrepare_ZerosIDAndSanitizesBody(t *testing.T) {
	c := Comment{ID: 99, Body: "  <script>alert(1)</script>  "}
	c.Prepare()

	if c.ID != 0 {
		t.Errorf("ID = %d, want 0", c.ID)
	}
	want := "&lt;script&gt;alert(1)&lt;/script&gt;"
	if c.Body != want {
		t.Errorf("Body = %q, want %q", c.Body, want)
	}
}

func TestCommentPrepare_SetsTimestamps(t *testing.T) {
	before := time.Now().Add(-time.Second)
	c := Comment{}
	c.Prepare()
	after := time.Now().Add(time.Second)

	if c.CreatedAt.Before(before) || c.CreatedAt.After(after) {
		t.Errorf("CreatedAt = %v, want between %v and %v", c.CreatedAt, before, after)
	}
	if c.UpdatedAt.Before(before) || c.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt = %v, want between %v and %v", c.UpdatedAt, before, after)
	}
}

func TestCommentValidate(t *testing.T) {
	tests := []struct {
		name    string
		comment Comment
		wantKey string
	}{
		{
			name:    "missing body",
			comment: Comment{UserID: 1, MatchupID: 1},
			wantKey: "Required_body",
		},
		{
			name:    "missing user",
			comment: Comment{Body: "hello", MatchupID: 1},
			wantKey: "Required_user",
		},
		{
			name:    "valid comment",
			comment: Comment{Body: "hello", UserID: 1, MatchupID: 1},
			wantKey: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := tc.comment.Validate("")
			if tc.wantKey == "" {
				if len(errs) != 0 {
					t.Errorf("expected no errors, got %v", errs)
				}
				return
			}
			if _, ok := errs[tc.wantKey]; !ok {
				t.Errorf("expected error key %q, got %v", tc.wantKey, errs)
			}
		})
	}
}
