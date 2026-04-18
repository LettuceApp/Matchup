package auth

import (
	"net/http"
	"strings"
	"testing"
)

func TestCreateToken(t *testing.T) {
	t.Setenv("API_SECRET", "test-secret-key")

	tests := []struct {
		name string
		id   uint
	}{
		{name: "user id 1", id: 1},
		{name: "user id 0", id: 0},
		{name: "large user id", id: 999999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := CreateToken(tt.id)
			if err != nil {
				t.Fatalf("CreateToken(%d) returned error: %v", tt.id, err)
			}
			parts := strings.Split(token, ".")
			if len(parts) != 3 {
				t.Errorf("CreateToken(%d) = %q; want 3-segment JWT (got %d segments)", tt.id, token, len(parts))
			}
		})
	}
}

func TestExtractToken(t *testing.T) {
	tests := []struct {
		name        string
		queryParam  string
		authHeader  string
		wantToken   string
	}{
		{
			name:       "from Authorization Bearer header",
			authHeader: "Bearer my-header-token",
			wantToken:  "my-header-token",
		},
		{
			name:       "from query param",
			queryParam: "my-query-token",
			wantToken:  "my-query-token",
		},
		{
			name:       "query param preferred over header",
			queryParam: "query-token",
			authHeader: "Bearer header-token",
			wantToken:  "query-token",
		},
		{
			name:      "empty when neither set",
			wantToken: "",
		},
		{
			name:       "empty with malformed Authorization header",
			authHeader: "InvalidHeaderNoSpace",
			wantToken:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "http://example.com/test"
			if tt.queryParam != "" {
				url += "?token=" + tt.queryParam
			}

			req, err := http.NewRequest(http.MethodGet, url, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			got := ExtractToken(req)
			if got != tt.wantToken {
				t.Errorf("ExtractToken() = %q; want %q", got, tt.wantToken)
			}
		})
	}
}

func TestExtractTokenID(t *testing.T) {
	t.Setenv("API_SECRET", "test-secret-key")

	t.Run("roundtrip with CreateToken", func(t *testing.T) {
		var wantID uint = 42
		token, err := CreateToken(wantID)
		if err != nil {
			t.Fatalf("CreateToken(%d) returned error: %v", wantID, err)
		}

		req, err := http.NewRequest(http.MethodGet, "http://example.com/test?token="+token, nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		gotID, err := ExtractTokenID(req)
		if err != nil {
			t.Fatalf("ExtractTokenID() returned error: %v", err)
		}
		if gotID != wantID {
			t.Errorf("ExtractTokenID() = %d; want %d", gotID, wantID)
		}
	})

	t.Run("rejects tampered token", func(t *testing.T) {
		token, err := CreateToken(42)
		if err != nil {
			t.Fatalf("CreateToken() returned error: %v", err)
		}

		// Flip a character in the signature (last segment)
		tampered := token[:len(token)-1] + "X"

		req, err := http.NewRequest(http.MethodGet, "http://example.com/test?token="+tampered, nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		_, err = ExtractTokenID(req)
		if err == nil {
			t.Errorf("ExtractTokenID() expected error for tampered token, got nil")
		}
	})

	t.Run("rejects token signed with wrong secret", func(t *testing.T) {
		token, err := CreateToken(42)
		if err != nil {
			t.Fatalf("CreateToken() returned error: %v", err)
		}

		// Switch to a different secret for validation
		t.Setenv("API_SECRET", "different-secret-key")

		req, err := http.NewRequest(http.MethodGet, "http://example.com/test?token="+token, nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		_, err = ExtractTokenID(req)
		if err == nil {
			t.Errorf("ExtractTokenID() expected error for wrong-secret token, got nil")
		}
	})
}

func TestTokenValid(t *testing.T) {
	t.Setenv("API_SECRET", "test-secret-key")

	t.Run("accepts valid token", func(t *testing.T) {
		token, err := CreateToken(1)
		if err != nil {
			t.Fatalf("CreateToken() returned error: %v", err)
		}

		req, err := http.NewRequest(http.MethodGet, "http://example.com/test?token="+token, nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		if err := TokenValid(req); err != nil {
			t.Errorf("TokenValid() returned error for valid token: %v", err)
		}
	})

	t.Run("rejects invalid token", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "http://example.com/test?token=not-a-valid-jwt", nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		if err := TokenValid(req); err == nil {
			t.Errorf("TokenValid() expected error for invalid token, got nil")
		}
	})
}
