//go:build integration

package api

import (
	"net/http"
	"strings"
	"testing"
)

// TestIntegration_SelfProtection_RefusesLockoutActions checks the API refuses the
// actions that would cut the caller off from the app when aimed at their own
// account. Hiding the buttons is not a control — these endpoints are reachable
// directly, and an admin holds the permission for them over every entry
// including their own.
func TestIntegration_SelfProtection_RefusesLockoutActions(t *testing.T) {
	env := setupTestServer(t)
	login := loginAs(t, env, "admin", "admin123")
	token := login.Token
	selfDN := login.Session.UserDN

	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"lock", http.MethodPost, userPath(selfDN) + "/lock", nil},
		{"delete", http.MethodDelete, userPath(selfDN), nil},
		{"remove password", http.MethodDelete, userPath(selfDN) + "/password", nil},
		{"rename", http.MethodPost, userPath(selfDN) + "/change-uid", map[string]string{"newUid": "admin-renamed"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := doJSON(t, env, tc.method, tc.path, tc.body, token)
			if resp.StatusCode != http.StatusConflict {
				t.Fatalf("status = %d, want 409 (body=%s)", resp.StatusCode, body)
			}
			if !strings.Contains(string(body), "signed in with") {
				t.Errorf("error does not explain why: %s", body)
			}
		})
	}

	// The account must be untouched: still present, still unlocked, still able
	// to authenticate.
	resp, body := doJSON(t, env, http.MethodGet, userPath(selfDN), nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("own account no longer readable: status=%d body=%s", resp.StatusCode, body)
	}
	if strings.Contains(string(body), `"accountLocked":true`) {
		t.Error("own account ended up locked")
	}
	loginAs(t, env, "admin", "admin123") // fatals if authentication broke
}

// TestIntegration_SelfProtection_OtherAccountsUnaffected pins that the guard is
// about the caller's own DN and nothing else — an admin must still be able to
// lock and delete other people.
func TestIntegration_SelfProtection_OtherAccountsUnaffected(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	user := createTestUser(t, env, token)

	if resp, body := doJSON(t, env, http.MethodPost, userPath(user.DN)+"/lock", nil, token); resp.StatusCode != http.StatusOK {
		t.Fatalf("lock another user: status=%d body=%s", resp.StatusCode, body)
	}
	if resp, body := doJSON(t, env, http.MethodDelete, userPath(user.DN), nil, token); resp.StatusCode != http.StatusOK {
		t.Fatalf("delete another user: status=%d body=%s", resp.StatusCode, body)
	}
}

// TestIntegration_SelfProtection_ChangeOwnPasswordStillWorks keeps the one
// self-service action that must stay available: rotating your own password.
func TestIntegration_SelfProtection_ChangeOwnPasswordStillWorks(t *testing.T) {
	env := setupTestServer(t)
	login := loginAs(t, env, "viewer", "viewer123")

	resp, body := doJSON(t, env, http.MethodPost, "/api/auth/change-password",
		map[string]string{"password": "viewer123"}, login.Token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("change own password: status=%d body=%s", resp.StatusCode, body)
	}

	// Same password back in place, so the fixture stays usable for other tests.
	loginAs(t, env, "viewer", "viewer123")
}
