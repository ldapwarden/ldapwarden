//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/auth"
)

func TestIntegration_Login_Admin(t *testing.T) {
	env := setupTestServer(t)

	resp, body := doJSON(t, env, http.MethodPost, "/api/auth/login",
		map[string]string{"username": "admin", "password": "admin123"}, "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", resp.StatusCode, body)
	}

	var lr auth.LoginResponse
	if err := json.Unmarshal(body, &lr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if lr.Token == "" {
		t.Error("token is empty")
	}
	if lr.Session == nil {
		t.Fatal("session is nil")
	}
	if lr.Session.UserUID != "admin" {
		t.Errorf("UserUID=%q, want admin", lr.Session.UserUID)
	}
	if lr.Session.RoleName != "admin" {
		t.Errorf("RoleName=%q, want admin", lr.Session.RoleName)
	}
	if !slices.Contains(lr.Session.Permissions, "users:write") {
		t.Errorf("admin missing users:write permission, got %v", lr.Session.Permissions)
	}
}

func TestIntegration_Login_ReadonlyUser(t *testing.T) {
	env := setupTestServer(t)
	lr := loginAs(t, env, "viewer", "viewer123")

	if lr.Session.RoleName != "readonly" {
		t.Errorf("RoleName=%q, want readonly", lr.Session.RoleName)
	}
	if slices.Contains(lr.Session.Permissions, "users:write") {
		t.Errorf("readonly should not have users:write, got %v", lr.Session.Permissions)
	}
	if !slices.Contains(lr.Session.Permissions, "users:read") {
		t.Errorf("readonly missing users:read, got %v", lr.Session.Permissions)
	}
}

func TestIntegration_Login_BadPassword(t *testing.T) {
	env := setupTestServer(t)
	resp, _ := doJSON(t, env, http.MethodPost, "/api/auth/login",
		map[string]string{"username": "admin", "password": "wrong-password"}, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", resp.StatusCode)
	}
}

func TestIntegration_Login_UnknownUser(t *testing.T) {
	env := setupTestServer(t)
	resp, _ := doJSON(t, env, http.MethodPost, "/api/auth/login",
		map[string]string{"username": "nosuchuser", "password": "irrelevant"}, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", resp.StatusCode)
	}
}

func TestIntegration_Login_MissingFields(t *testing.T) {
	env := setupTestServer(t)
	resp, _ := doJSON(t, env, http.MethodPost, "/api/auth/login",
		map[string]string{"username": "admin"}, "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", resp.StatusCode)
	}
}

func TestIntegration_Me_AfterLogin(t *testing.T) {
	env := setupTestServer(t)
	lr := loginAs(t, env, "admin", "admin123")

	resp, body := doJSON(t, env, http.MethodGet, "/api/auth/me", nil, lr.Token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", resp.StatusCode, body)
	}

	var session auth.Session
	if err := json.Unmarshal(body, &session); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if session.UserUID != "admin" {
		t.Errorf("UserUID=%q, want admin", session.UserUID)
	}
	if session.RoleName != lr.Session.RoleName {
		t.Errorf("RoleName drift: /me=%q, login=%q", session.RoleName, lr.Session.RoleName)
	}
}

func TestIntegration_Me_NoToken(t *testing.T) {
	env := setupTestServer(t)
	resp, _ := doJSON(t, env, http.MethodGet, "/api/auth/me", nil, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", resp.StatusCode)
	}
}

func TestIntegration_Me_InvalidToken(t *testing.T) {
	env := setupTestServer(t)
	resp, _ := doJSON(t, env, http.MethodGet, "/api/auth/me", nil, "not-a-real-token")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", resp.StatusCode)
	}
}

func TestIntegration_Logout_InvalidatesToken(t *testing.T) {
	env := setupTestServer(t)
	lr := loginAs(t, env, "admin", "admin123")

	resp, body := doJSON(t, env, http.MethodPost, "/api/auth/logout", nil, lr.Token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout status=%d, want 200; body=%s", resp.StatusCode, body)
	}

	resp2, _ := doJSON(t, env, http.MethodGet, "/api/auth/me", nil, lr.Token)
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("/me after logout status=%d, want 401", resp2.StatusCode)
	}
}
