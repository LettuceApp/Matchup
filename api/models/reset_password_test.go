package models

import (
	"testing"
)

func TestResetPasswordPrepare_SanitizesFields(t *testing.T) {
	rp := ResetPassword{
		Email: "  Test@Example.COM  ",
		Token: "  abc123  ",
	}
	rp.Prepare()

	wantEmail := "Test@Example.COM"
	if rp.Email != wantEmail {
		t.Errorf("Email = %q, want %q", rp.Email, wantEmail)
	}
	wantToken := "abc123"
	if rp.Token != wantToken {
		t.Errorf("Token = %q, want %q", rp.Token, wantToken)
	}
}

func TestResetPasswordPrepare_HandlesEmptyStrings(t *testing.T) {
	rp := ResetPassword{Email: "", Token: ""}
	rp.Prepare()

	if rp.Email != "" {
		t.Errorf("Email = %q, want empty", rp.Email)
	}
	if rp.Token != "" {
		t.Errorf("Token = %q, want empty", rp.Token)
	}
}
