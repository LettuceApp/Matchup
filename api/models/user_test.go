package models

import (
	"strings"
	"testing"
	"time"
)

// Prepare() used to call html.EscapeString on username + email but
// React auto-escapes JSX text nodes at render — the backend escape
// was double-encoding and would have surfaced as &#39; etc. for any
// username with an apostrophe. The tests now assert the new contract:
// trim + lowercase, no escape.
func TestUserPrepare_TrimsAndLowercasesUsername(t *testing.T) {
	u := User{Username: "  TestUser  "}
	u.Prepare()
	want := "testuser"
	if u.Username != want {
		t.Errorf("Username = %q, want %q", u.Username, want)
	}
}

func TestUserPrepare_TrimsAndLowercasesEmail(t *testing.T) {
	u := User{Email: "  Test@Example.COM  "}
	u.Prepare()
	want := "test@example.com"
	if u.Email != want {
		t.Errorf("Email = %q, want %q", u.Email, want)
	}
}

// Locks in the round-trip behavior for the case that surfaced the
// double-encode bug for Bracket / Matchup. Mirrors
// TestBracketPrepare_PreservesApostrophe.
func TestUserPrepare_PreservesApostropheInUsername(t *testing.T) {
	u := User{Username: "O'Brien"}
	u.Prepare()
	want := "o'brien"
	if u.Username != want {
		t.Errorf("Username = %q, want %q (apostrophe must round-trip un-encoded)",
			u.Username, want)
	}
}

func TestUserPrepare_PreservesApostropheInEmail(t *testing.T) {
	// O'Brien-style emails are valid per RFC 5321 (local-part may
	// contain quoted strings or limited punctuation). Confirm the
	// backend doesn't mangle them.
	u := User{Email: "o'brien@example.com"}
	u.Prepare()
	want := "o'brien@example.com"
	if u.Email != want {
		t.Errorf("Email = %q, want %q (apostrophe must round-trip un-encoded)",
			u.Email, want)
	}
}

func TestUserPrepare_SetsTimestamps(t *testing.T) {
	before := time.Now().Add(-time.Second)
	u := User{}
	u.Prepare()
	after := time.Now().Add(time.Second)

	if u.CreatedAt.Before(before) || u.CreatedAt.After(after) {
		t.Errorf("CreatedAt = %v, want between %v and %v", u.CreatedAt, before, after)
	}
	if u.UpdatedAt.Before(before) || u.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt = %v, want between %v and %v", u.UpdatedAt, before, after)
	}
}

func TestUserPrepare_SetsIsAdminForSeedEmail(t *testing.T) {
	u := User{Email: strings.ToUpper(SeedAdminEmail)}
	u.Prepare()
	if !u.IsAdmin {
		t.Errorf("IsAdmin = false, want true for seed admin email")
	}
}

func TestUserPrepare_SetsIsAdminFalseForNonSeedNewUser(t *testing.T) {
	u := User{ID: 0, Email: "other@example.com", IsAdmin: true}
	u.Prepare()
	if u.IsAdmin {
		t.Errorf("IsAdmin = true, want false for non-seed new user")
	}
}

func TestUserValidate(t *testing.T) {
	tests := []struct {
		name    string
		user    User
		action  string
		wantKey string
	}{
		{
			name:    "default missing username",
			user:    User{Password: "password123", Email: "a@b.com"},
			action:  "",
			wantKey: "Required_username",
		},
		{
			name:    "default missing password",
			user:    User{Username: "alice", Email: "a@b.com"},
			action:  "",
			wantKey: "Required_password",
		},
		{
			name:    "default short password",
			user:    User{Username: "alice", Password: "abc", Email: "a@b.com"},
			action:  "",
			wantKey: "Invalid_password",
		},
		{
			name:    "default invalid email",
			user:    User{Username: "alice", Password: "password123", Email: "bad-email"},
			action:  "",
			wantKey: "Invalid_email",
		},
		{
			name:    "default valid user",
			user:    User{Username: "alice", Password: "password123", Email: "a@b.com"},
			action:  "",
			wantKey: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := tc.user.Validate(tc.action)
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
