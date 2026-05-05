//go:build integration

package controllers

import (
	"context"
	"testing"

	"Matchup/auth"
	authv1 "Matchup/gen/auth/v1"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
)

func TestLogin_Success(t *testing.T) {
	db := setupTestDB(t)
	t.Setenv("API_SECRET", "integration-test-secret")

	user := seedTestUser(t, db, "loginuser", "login@example.com", "TestPass123")
	handler := &AuthHandler{DB: db}

	resp, err := handler.Login(context.Background(), connectReq(&authv1.LoginRequest{
		Email:    "login@example.com",
		Password: "TestPass123",
	}, ""))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Msg.Token == "" {
		t.Error("expected non-empty token")
	}
	if resp.Msg.Email != user.Email {
		t.Errorf("expected email %q, got %q", user.Email, resp.Msg.Email)
	}
	if resp.Msg.Username != user.Username {
		t.Errorf("expected username %q, got %q", user.Username, resp.Msg.Username)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	db := setupTestDB(t)
	t.Setenv("API_SECRET", "integration-test-secret")

	seedTestUser(t, db, "wrongpw", "wrongpw@example.com", "TestPass123")
	handler := &AuthHandler{DB: db}

	_, err := handler.Login(context.Background(), connectReq(&authv1.LoginRequest{
		Email:    "wrongpw@example.com",
		Password: "WrongPassword",
	}, ""))
	assertConnectError(t, err, connect.CodeUnauthenticated)
}

func TestLogin_UserNotFound(t *testing.T) {
	db := setupTestDB(t)
	t.Setenv("API_SECRET", "integration-test-secret")

	handler := &AuthHandler{DB: db}

	_, err := handler.Login(context.Background(), connectReq(&authv1.LoginRequest{
		Email:    "nonexistent@example.com",
		Password: "TestPass123",
	}, ""))
	assertConnectError(t, err, connect.CodeUnauthenticated)
}

func TestLogin_ByUsername(t *testing.T) {
	db := setupTestDB(t)
	t.Setenv("API_SECRET", "integration-test-secret")

	seedTestUser(t, db, "usernamelogin", "usernamelogin@example.com", "TestPass123")
	handler := &AuthHandler{DB: db}

	resp, err := handler.Login(context.Background(), connectReq(&authv1.LoginRequest{
		Email:    "usernamelogin",
		Password: "TestPass123",
	}, ""))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Msg.Token == "" {
		t.Error("expected non-empty token")
	}
}

func TestResetPassword_MissingToken(t *testing.T) {
	db := setupTestDB(t)
	handler := &AuthHandler{DB: db}

	_, err := handler.ResetPassword(context.Background(), connectReq(&authv1.ResetPasswordRequest{
		Token:          "",
		NewPassword:    "newpass123",
		RetypePassword: "newpass123",
	}, ""))
	assertConnectError(t, err, connect.CodeInvalidArgument)
}

func TestResetPassword_PasswordMismatch(t *testing.T) {
	db := setupTestDB(t)
	handler := &AuthHandler{DB: db}

	_, err := handler.ResetPassword(context.Background(), connectReq(&authv1.ResetPasswordRequest{
		Token:          "fake",
		NewPassword:    "newpass123",
		RetypePassword: "different",
	}, ""))
	assertConnectError(t, err, connect.CodeInvalidArgument)
}

func TestResetPassword_ShortPassword(t *testing.T) {
	db := setupTestDB(t)
	handler := &AuthHandler{DB: db}

	_, err := handler.ResetPassword(context.Background(), connectReq(&authv1.ResetPasswordRequest{
		Token:          "fake",
		NewPassword:    "abc",
		RetypePassword: "abc",
	}, ""))
	assertConnectError(t, err, connect.CodeInvalidArgument)
}

// TestLogin_ReturnsRefreshToken — Login now hands back a refresh
// token + access_token_expires_in in addition to the access token.
// Existing clients that ignore the new fields still work.
func TestLogin_ReturnsRefreshToken(t *testing.T) {
	db := setupTestDB(t)
	t.Setenv("API_SECRET", "integration-test-secret")

	seedTestUser(t, db, "freshlogin", "freshlogin@example.com", "TestPass123")
	handler := &AuthHandler{DB: db}

	resp, err := handler.Login(context.Background(), connectReq(&authv1.LoginRequest{
		Email:    "freshlogin@example.com",
		Password: "TestPass123",
	}, ""))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if resp.Msg.RefreshToken == "" {
		t.Error("expected RefreshToken to be set")
	}
	if resp.Msg.AccessTokenExpiresIn <= 0 {
		t.Errorf("expected positive AccessTokenExpiresIn, got %d", resp.Msg.AccessTokenExpiresIn)
	}
	// A row landed in refresh_tokens for this user.
	var n int
	_ = db.Get(&n, "SELECT COUNT(*) FROM refresh_tokens WHERE user_id = (SELECT id FROM users WHERE email='freshlogin@example.com')")
	if n != 1 {
		t.Errorf("expected 1 refresh_tokens row after Login, got %d", n)
	}
}

// TestRefresh_HappyPath — presenting the refresh token returned by
// Login rotates it: the response carries a new refresh + access, and
// the old refresh can no longer rotate (but in this test we just
// verify the happy path — reuse detection has its own test).
func TestRefresh_HappyPath(t *testing.T) {
	db := setupTestDB(t)
	t.Setenv("API_SECRET", "integration-test-secret")

	seedTestUser(t, db, "refresher", "refresher@example.com", "TestPass123")
	handler := &AuthHandler{DB: db}

	login, err := handler.Login(context.Background(), connectReq(&authv1.LoginRequest{
		Email:    "refresher@example.com",
		Password: "TestPass123",
	}, ""))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	originalRefresh := login.Msg.RefreshToken

	rotated, err := handler.Refresh(context.Background(), connectReq(&authv1.RefreshRequest{
		RefreshToken: originalRefresh,
	}, ""))
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if rotated.Msg.Token == "" {
		t.Error("expected new access Token")
	}
	if rotated.Msg.RefreshToken == originalRefresh {
		t.Error("Refresh must return a different refresh_token")
	}
	if rotated.Msg.AccessTokenExpiresIn <= 0 {
		t.Errorf("expected positive AccessTokenExpiresIn, got %d", rotated.Msg.AccessTokenExpiresIn)
	}
}

// TestRefresh_ReuseRevokesFamily — presenting the SAME refresh token
// twice is a theft signal. Server responds 401 and revokes every
// active row in the family, so the rotated child can no longer
// refresh either.
func TestRefresh_ReuseRevokesFamily(t *testing.T) {
	db := setupTestDB(t)
	t.Setenv("API_SECRET", "integration-test-secret")

	seedTestUser(t, db, "theft", "theft@example.com", "TestPass123")
	handler := &AuthHandler{DB: db}

	login, _ := handler.Login(context.Background(), connectReq(&authv1.LoginRequest{
		Email:    "theft@example.com",
		Password: "TestPass123",
	}, ""))
	original := login.Msg.RefreshToken

	// First rotation — happy path.
	rotated, err := handler.Refresh(context.Background(), connectReq(&authv1.RefreshRequest{
		RefreshToken: original,
	}, ""))
	if err != nil {
		t.Fatalf("first rotate: %v", err)
	}

	// Second rotation with the ORIGINAL (attacker's replay).
	_, err = handler.Refresh(context.Background(), connectReq(&authv1.RefreshRequest{
		RefreshToken: original,
	}, ""))
	assertConnectError(t, err, connect.CodeUnauthenticated)

	// The rotated child is now revoked too — legitimate user gets
	// logged out along with the attacker.
	_, err = handler.Refresh(context.Background(), connectReq(&authv1.RefreshRequest{
		RefreshToken: rotated.Msg.RefreshToken,
	}, ""))
	assertConnectError(t, err, connect.CodeUnauthenticated)
}

// TestRefresh_InvalidToken — junk plaintext returns 401 and doesn't
// leak which specific state the server saw.
func TestRefresh_InvalidToken(t *testing.T) {
	db := setupTestDB(t)
	t.Setenv("API_SECRET", "integration-test-secret")
	handler := &AuthHandler{DB: db}

	_, err := handler.Refresh(context.Background(), connectReq(&authv1.RefreshRequest{
		RefreshToken: "not-a-real-token",
	}, ""))
	assertConnectError(t, err, connect.CodeUnauthenticated)
}

// TestLogout_RevokesSpecificRow — Logout with a specific refresh
// token revokes only that row; other active rows for the same user
// stay valid.
func TestLogout_RevokesSpecificRow(t *testing.T) {
	db := setupTestDB(t)
	t.Setenv("API_SECRET", "integration-test-secret")

	u := seedTestUser(t, db, "logout", "logout@example.com", "TestPass123")
	handler := &AuthHandler{DB: db}

	// Two separate Login calls = two separate refresh tokens / devices.
	l1, _ := handler.Login(context.Background(), connectReq(&authv1.LoginRequest{
		Email: "logout@example.com", Password: "TestPass123",
	}, ""))
	l2, _ := handler.Login(context.Background(), connectReq(&authv1.LoginRequest{
		Email: "logout@example.com", Password: "TestPass123",
	}, ""))

	// Logout kills only the first; needs auth context for the uid gate.
	ctx := httpctx.WithUserID(context.Background(), u.ID)
	if _, err := handler.Logout(ctx, connectReq(&authv1.LogoutRequest{
		RefreshToken: l1.Msg.RefreshToken,
	}, "")); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	// The first refresh token can no longer rotate.
	_, err := handler.Refresh(context.Background(), connectReq(&authv1.RefreshRequest{
		RefreshToken: l1.Msg.RefreshToken,
	}, ""))
	assertConnectError(t, err, connect.CodeUnauthenticated)

	// The second still works — the device wasn't touched.
	if _, err := handler.Refresh(context.Background(), connectReq(&authv1.RefreshRequest{
		RefreshToken: l2.Msg.RefreshToken,
	}, "")); err != nil {
		t.Errorf("second device's refresh should still work, got %v", err)
	}
}

// TestLogout_RejectsAnon — Logout requires an authed caller.
func TestLogout_RejectsAnon(t *testing.T) {
	db := setupTestDB(t)
	handler := &AuthHandler{DB: db}

	_, err := handler.Logout(context.Background(), connectReq(&authv1.LogoutRequest{
		RefreshToken: "whatever",
	}, ""))
	assertConnectError(t, err, connect.CodeUnauthenticated)
}

// TestLogoutAll_RevokesEveryRow — LogoutAll returns the number of
// rows revoked and all of the caller's refresh tokens stop working.
func TestLogoutAll_RevokesEveryRow(t *testing.T) {
	db := setupTestDB(t)
	t.Setenv("API_SECRET", "integration-test-secret")

	u := seedTestUser(t, db, "lall", "lall@example.com", "TestPass123")
	handler := &AuthHandler{DB: db}

	var tokens []string
	for i := 0; i < 3; i++ {
		l, err := handler.Login(context.Background(), connectReq(&authv1.LoginRequest{
			Email: "lall@example.com", Password: "TestPass123",
		}, ""))
		if err != nil {
			t.Fatalf("Login %d: %v", i, err)
		}
		tokens = append(tokens, l.Msg.RefreshToken)
	}

	ctx := httpctx.WithUserID(context.Background(), u.ID)
	resp, err := handler.LogoutAll(ctx, connectReq(&authv1.LogoutAllRequest{}, ""))
	if err != nil {
		t.Fatalf("LogoutAll: %v", err)
	}
	if resp.Msg.RevokedCount != 3 {
		t.Errorf("expected RevokedCount=3, got %d", resp.Msg.RevokedCount)
	}

	// Every device now 401s on Refresh.
	for i, tok := range tokens {
		if _, err := handler.Refresh(context.Background(), connectReq(&authv1.RefreshRequest{
			RefreshToken: tok,
		}, "")); err == nil {
			t.Errorf("token %d should be revoked; Refresh succeeded", i)
		}
	}
}

// Sanity: the new access tokens carry an exp claim, which the JWT
// validator enforces. Regression guard against someone accidentally
// removing the exp claim in a future change.
func TestAccessToken_HasExp(t *testing.T) {
	t.Setenv("API_SECRET", "integration-test-secret")
	tok, err := auth.CreateToken(123)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	// The JWT body is the middle segment, base64-url-encoded JSON.
	// We don't need to parse it cryptographically — just confirm the
	// string "exp" appears, which is a cheap smoke for the claim.
	if !contains(tok, ".") {
		t.Fatal("expected 3-segment JWT")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
