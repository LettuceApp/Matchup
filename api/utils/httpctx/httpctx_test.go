package httpctx

import (
	"context"
	"testing"
)

func TestWithUserID_CurrentUserID_Roundtrip(t *testing.T) {
	tests := []struct {
		name string
		id   uint
	}{
		{"typical id", 42},
		{"zero id", 0},
		{"large id", 999999},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := WithUserID(context.Background(), tc.id)
			got, ok := CurrentUserID(ctx)
			if !ok {
				t.Errorf("CurrentUserID returned ok=false, want true")
			}
			if got != tc.id {
				t.Errorf("CurrentUserID = %d, want %d", got, tc.id)
			}
		})
	}
}

func TestCurrentUserID_EmptyContext(t *testing.T) {
	got, ok := CurrentUserID(context.Background())
	if ok {
		t.Errorf("CurrentUserID on empty context returned ok=true, want false")
	}
	if got != 0 {
		t.Errorf("CurrentUserID on empty context = %d, want 0", got)
	}
}

func TestWithIsAdmin_IsAdminRequest_Roundtrip(t *testing.T) {
	tests := []struct {
		name    string
		isAdmin bool
	}{
		{"admin true", true},
		{"admin false", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := WithIsAdmin(context.Background(), tc.isAdmin)
			got := IsAdminRequest(ctx)
			if got != tc.isAdmin {
				t.Errorf("IsAdminRequest = %v, want %v", got, tc.isAdmin)
			}
		})
	}
}

func TestIsAdminRequest_EmptyContext(t *testing.T) {
	got := IsAdminRequest(context.Background())
	if got {
		t.Errorf("IsAdminRequest on empty context = true, want false")
	}
}

func TestWithReadPrimary_ShouldReadPrimary_Roundtrip(t *testing.T) {
	ctx := WithReadPrimary(context.Background())
	got := ShouldReadPrimary(ctx)
	if !got {
		t.Errorf("ShouldReadPrimary = false, want true")
	}
}

func TestShouldReadPrimary_EmptyContext(t *testing.T) {
	got := ShouldReadPrimary(context.Background())
	if got {
		t.Errorf("ShouldReadPrimary on empty context = true, want false")
	}
}
