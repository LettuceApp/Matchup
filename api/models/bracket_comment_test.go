package models

import (
	"testing"
	"time"
)

func TestBracketCommentPrepare_ZerosIDAndSanitizesBody(t *testing.T) {
	c := BracketComment{ID: 42, Body: "  <em>hi</em>  "}
	c.Prepare()

	if c.ID != 0 {
		t.Errorf("ID = %d, want 0", c.ID)
	}
	want := "&lt;em&gt;hi&lt;/em&gt;"
	if c.Body != want {
		t.Errorf("Body = %q, want %q", c.Body, want)
	}
}

func TestBracketCommentPrepare_SetsTimestamps(t *testing.T) {
	before := time.Now().Add(-time.Second)
	c := BracketComment{}
	c.Prepare()
	after := time.Now().Add(time.Second)

	if c.CreatedAt.Before(before) || c.CreatedAt.After(after) {
		t.Errorf("CreatedAt = %v, want between %v and %v", c.CreatedAt, before, after)
	}
	if c.UpdatedAt.Before(before) || c.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt = %v, want between %v and %v", c.UpdatedAt, before, after)
	}
}

func TestBracketCommentValidate(t *testing.T) {
	tests := []struct {
		name    string
		comment BracketComment
		wantKey string
	}{
		{
			name:    "missing body",
			comment: BracketComment{UserID: 1, BracketID: 1},
			wantKey: "Required_body",
		},
		{
			name:    "missing bracket",
			comment: BracketComment{Body: "hello", UserID: 1},
			wantKey: "Required_bracket",
		},
		{
			name:    "valid bracket comment",
			comment: BracketComment{Body: "hello", UserID: 1, BracketID: 1},
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
