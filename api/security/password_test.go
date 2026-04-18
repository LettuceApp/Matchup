package security

import (
	"strings"
	"testing"
)

func TestHash(t *testing.T) {
	tests := []struct {
		name     string
		password string
	}{
		{name: "valid bcrypt output", password: "mysecretpassword"},
		{name: "empty string input", password: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hashed, err := Hash(tt.password)
			if err != nil {
				t.Fatalf("Hash(%q) returned error: %v", tt.password, err)
			}
			if len(hashed) == 0 {
				t.Errorf("Hash(%q) returned empty result", tt.password)
			}
			if !strings.HasPrefix(string(hashed), "$2a$") {
				t.Errorf("Hash(%q) = %q; want prefix $2a$", tt.password, string(hashed))
			}
		})
	}
}

func TestHashDifferentInputs(t *testing.T) {
	hash1, err := Hash("password1")
	if err != nil {
		t.Fatalf("Hash(password1) returned error: %v", err)
	}
	hash2, err := Hash("password2")
	if err != nil {
		t.Fatalf("Hash(password2) returned error: %v", err)
	}
	if string(hash1) == string(hash2) {
		t.Errorf("Hash produced identical output for different inputs")
	}
}

func TestVerifyPassword(t *testing.T) {
	tests := []struct {
		name      string
		password  string
		check     string
		wantError bool
	}{
		{
			name:      "correct password succeeds",
			password:  "correcthorsebatterystaple",
			check:     "correcthorsebatterystaple",
			wantError: false,
		},
		{
			name:      "wrong password fails",
			password:  "correcthorsebatterystaple",
			check:     "wrongpassword",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hashed, err := Hash(tt.password)
			if err != nil {
				t.Fatalf("Hash(%q) returned error: %v", tt.password, err)
			}

			err = VerifyPassword(string(hashed), tt.check)
			if tt.wantError && err == nil {
				t.Errorf("VerifyPassword() expected error for wrong password, got nil")
			}
			if !tt.wantError && err != nil {
				t.Errorf("VerifyPassword() expected nil error for correct password, got %v", err)
			}
		})
	}
}
