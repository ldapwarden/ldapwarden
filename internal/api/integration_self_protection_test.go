//go:build integration

package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// TestIntegration_SelfProtection_RefusesLockoutActions checks the API refuses the
// actions that would cut the caller off from the app when aimed at their own
// account. Hiding the buttons is not a control — these endpoints are reachable
// directly, and an admin holds the permission for them over every entry
// including their own.
func TestIntegration_SelfProtection_RefusesLockoutActions(t *testing.T) {
	env := setupTestServer(t)
	// A throwaway admin, not the seeded one: these requests are meant to attack
	// the caller's own account, and a test must not leave the shared fixture
	// altered.
	self, token := createTestAdmin(t, env, loginAs(t, env, "admin", "admin123").Token)
	selfDN := self.DN

	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"lock", http.MethodPost, userPath(selfDN) + "/lock", nil},
		{"delete", http.MethodDelete, userPath(selfDN), nil},
		{"remove password", http.MethodDelete, userPath(selfDN) + "/password", nil},
		{"rename", http.MethodPost, userPath(selfDN) + "/change-uid", map[string]string{"newUid": "renamed-" + self.UID}},
		// Both of these write an expiry rather than a lock, which is the same
		// lockout by another name: pwdAccountLockedTime is the attribute `lock`
		// itself uses, and shadowExpire is what the status column reads.
		{"expire", http.MethodPut, userPath(selfDN) + "/expiration", map[string]string{"expirationDate": "2020-01-01"}},
		{"shadow expiry", http.MethodPut, userPath(selfDN) + "/shadow", map[string]any{"shadowExpire": 1}},
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
	// Still able to authenticate: the guard has to leave the account usable.
	if resp, body := doJSON(t, env, http.MethodGet, "/api/auth/me", nil, token); resp.StatusCode != http.StatusOK {
		t.Fatalf("own session no longer valid: status=%d body=%s", resp.StatusCode, body)
	}
}

// TestIntegration_SelfProtection_ClearingOwnExpiryAllowed keeps the escape hatch
// open: a date already on your account must be removable, otherwise an operator
// who inherits an expiring admin account has no way back.
func TestIntegration_SelfProtection_ClearingOwnExpiryAllowed(t *testing.T) {
	env := setupTestServer(t)
	self, token := createTestAdmin(t, env, loginAs(t, env, "admin", "admin123").Token)
	selfDN := self.DN

	// Clearing when nothing is set is a no-op, which is exactly the shape of the
	// request the UI sends for "Clear expiration".
	resp, body := doJSON(t, env, http.MethodPut, userPath(selfDN)+"/expiration",
		map[string]string{"expirationDate": ""}, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clearing own expiration: status=%d body=%s", resp.StatusCode, body)
	}

	// Neutral shadow edits stay available too — only shadowExpire is refused.
	resp, body = doJSON(t, env, http.MethodPut, userPath(selfDN)+"/shadow",
		map[string]any{"shadowWarning": 7}, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("neutral shadow edit on own account: status=%d body=%s", resp.StatusCode, body)
	}
	if resp, body := doJSON(t, env, http.MethodPut, userPath(selfDN)+"/shadow",
		map[string]any{"shadowExpire": ldap.ShadowExpireNever}, token); resp.StatusCode != http.StatusOK {
		t.Fatalf("clearing own shadow expiry: status=%d body=%s", resp.StatusCode, body)
	}
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
