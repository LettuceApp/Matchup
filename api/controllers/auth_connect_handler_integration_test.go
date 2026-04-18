//go:build integration

package controllers

import (
	"context"
	"testing"

	authv1 "Matchup/gen/auth/v1"

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
