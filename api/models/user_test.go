package models

import (
	"strings"
	"testing"
	"time"
)

func TestUserPrepare_SanitizesUsername(t *testing.T) {
	u := User{Username: "  TestUser  "}
	u.Prepare()
	want := "testuser"
	if u.Username != want {
		t.Errorf("Username = %q, want %q", u.Username, want)
	}
}

func TestUserPrepare_SanitizesEmail(t *testing.T) {
	u := User{Email: "  Test@Example.COM  "}
	u.Prepare()
	want := "test@example.com"
	if u.Email != want {
		t.Errorf("Email = %q, want %q", u.Email, want)
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
