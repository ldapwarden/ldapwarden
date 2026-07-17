//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// TestIntegration_Users_CRUDLifecycle exercises create → get → update →
// lock → unlock → password change → delete on a fresh user.
func TestIntegration_Users_CRUDLifecycle(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	user := createTestUser(t, env, token)

	// GET it back.
	resp, body := doJSON(t, env, http.MethodGet, userPath(user.DN), nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get user: status=%d body=%s", resp.StatusCode, body)
	}
	var got ldap.User
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.UID != user.UID {
		t.Errorf("UID=%q, want %q", got.UID, user.UID)
	}
	if got.Mail != user.Mail {
		t.Errorf("Mail=%q, want %q", got.Mail, user.Mail)
	}

	// Update mail and sn.
	newMail := "updated-" + user.UID + "@test.example"
	newSN := "Updated-" + user.UID
	resp, body = doJSON(t, env, http.MethodPut, userPath(user.DN), map[string]any{
		"mail": newMail,
		"sn":   newSN,
	}, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update user: status=%d body=%s", resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal updated: %v", err)
	}
	if got.Mail != newMail {
		t.Errorf("Mail after update=%q, want %q", got.Mail, newMail)
	}
	if got.SN != newSN {
		t.Errorf("SN after update=%q, want %q", got.SN, newSN)
	}

	// Sanity-check: the original password binds before we touch anything.
	resp, _ = doJSON(t, env, http.MethodPost, "/api/auth/login",
		map[string]string{"username": user.UID, "password": "testpass"}, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login with original password before lock: status=%d, want 200", resp.StatusCode)
	}

	// Lock.
	resp, _ = doJSON(t, env, http.MethodPost, userPath(user.DN)+"/lock", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("lock: status=%d", resp.StatusCode)
	}
	resp, body = doJSON(t, env, http.MethodGet, userPath(user.DN), nil, token)
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal locked: %v", err)
	}
	if !got.AccountLocked {
		t.Errorf("AccountLocked=false after lock")
	}
	// Bind must fail while the account is locked.
	resp, _ = doJSON(t, env, http.MethodPost, "/api/auth/login",
		map[string]string{"username": user.UID, "password": "testpass"}, "")
	if resp.StatusCode == http.StatusOK {
		t.Errorf("login while locked: status=200, want non-200")
	}

	// Unlock.
	resp, _ = doJSON(t, env, http.MethodPost, userPath(user.DN)+"/unlock", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unlock: status=%d", resp.StatusCode)
	}
	resp, body = doJSON(t, env, http.MethodGet, userPath(user.DN), nil, token)
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal unlocked: %v", err)
	}
	if got.AccountLocked {
		t.Errorf("AccountLocked=true after unlock")
	}
	// The original password must still work after a lock/unlock cycle (C2
	// regression check: the previous implementation prefixed userPassword
	// with "!", which got re-hashed by the ppolicy hash-cleartext overlay
	// and could not be recovered on unlock).
	resp, _ = doJSON(t, env, http.MethodPost, "/api/auth/login",
		map[string]string{"username": user.UID, "password": "testpass"}, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login with original password after unlock: status=%d, want 200", resp.StatusCode)
	}

	// Change password, verify by binding.
	newPass := "rotated-pass-456"
	resp, body = doJSON(t, env, http.MethodPost, userPath(user.DN)+"/password",
		map[string]string{"password": newPass}, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("change password: status=%d body=%s", resp.StatusCode, body)
	}
	resp, _ = doJSON(t, env, http.MethodPost, "/api/auth/login",
		map[string]string{"username": user.UID, "password": newPass}, "")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("login with new password: status=%d, want 200", resp.StatusCode)
	}

	// Delete (cleanup would do it too but we want to assert the response).
	resp, _ = doJSON(t, env, http.MethodDelete, userPath(user.DN), nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete: status=%d", resp.StatusCode)
	}
	resp, _ = doJSON(t, env, http.MethodGet, userPath(user.DN), nil, token)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("get after delete: status=%d, want 404", resp.StatusCode)
	}
}

func TestIntegration_Users_CreateRequiresFields(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	// Missing uid.
	resp, _ := doJSON(t, env, http.MethodPost, "/api/users/", map[string]any{
		"givenName": "X", "sn": "Y", "uidNumber": 60001, "gidNumber": 60001,
	}, token)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing uid: status=%d, want 400", resp.StatusCode)
	}

	// Missing uidNumber.
	resp, _ = doJSON(t, env, http.MethodPost, "/api/users/", map[string]any{
		"uid": "x", "givenName": "X", "sn": "Y", "gidNumber": 60001,
	}, token)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing uidNumber: status=%d, want 400", resp.StatusCode)
	}
}

// TestIntegration_Users_CreateRejectsUnsafeUID verifies that UIDs containing
// characters that would corrupt or shift the resulting DN are rejected at the
// API boundary — closes the C3 finding from REPORT.md (DN injection at
// entry creation).
func TestIntegration_Users_CreateRejectsUnsafeUID(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	cases := []string{
		"alice,ou=Admins",  // would shift the DN to a different OU
		"alice+uid=root",   // RDN multivalued
		"alice;injected",   // RFC 4514 separator
		"alice space",      // space in the middle
		" alice",           // leading space
		"#alice",           // leading #
		"alice\\backslash", // literal backslash
		"alice\nnewline",   // CRLF in attribute
	}
	for _, uid := range cases {
		t.Run(uid, func(t *testing.T) {
			resp, _ := doJSON(t, env, http.MethodPost, "/api/users/", map[string]any{
				"uid":       uid,
				"givenName": "X",
				"sn":        "Y",
				"uidNumber": 60050,
				"gidNumber": 60050,
			}, token)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("uid=%q: status=%d, want 400", uid, resp.StatusCode)
			}
		})
	}
}

func TestIntegration_Users_CreateDuplicate(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	user := createTestUser(t, env, token)

	// Re-create with the same uid → LDAP rejects with "entry already exists",
	// which the handler surfaces as 409 Conflict.
	resp, _ := doJSON(t, env, http.MethodPost, "/api/users/", map[string]any{
		"uid":       user.UID,
		"givenName": "Dup",
		"sn":        "Dup",
		"uidNumber": user.UIDNumber + 1,
		"gidNumber": user.GIDNumber + 1,
	}, token)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate uid: status=%d, want 409", resp.StatusCode)
	}
}

func TestIntegration_Users_GetMissing(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	resp, _ := doJSON(t, env, http.MethodGet,
		userPath("uid=nosuch,ou=People,dc=example,dc=org"), nil, token)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status=%d, want 404", resp.StatusCode)
	}
}

func TestIntegration_Users_RequiresAuth(t *testing.T) {
	env := setupTestServer(t)

	resp, _ := doJSON(t, env, http.MethodGet, "/api/users/", nil, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401", resp.StatusCode)
	}
}

// TestIntegration_Users_DNScopeEnforced verifies that the {dn} URL parameter
// is rejected when it points outside the user OU — closes the C1 finding from
// REPORT.md (a user with users:read could otherwise reach arbitrary entries
// such as the bind admin DN).
func TestIntegration_Users_DNScopeEnforced(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	cases := []struct {
		name string
		dn   string
	}{
		{"bind admin", "cn=admin,dc=example,dc=org"},
		{"different OU", "cn=admins,ou=Groups,dc=example,dc=org"},
		{"base DN itself", "dc=example,dc=org"},
		{"unparseable", "not a dn"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, _ := doJSON(t, env, http.MethodGet, userPath(tc.dn), nil, token)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("GET %s: status=%d, want 400", tc.dn, resp.StatusCode)
			}
			resp, _ = doJSON(t, env, http.MethodDelete, userPath(tc.dn), nil, token)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("DELETE %s: status=%d, want 400", tc.dn, resp.StatusCode)
			}
		})
	}
}

func TestIntegration_Users_ReadonlyCannotWrite(t *testing.T) {
	env := setupTestServer(t)
	viewerToken := loginAs(t, env, "viewer", "viewer123").Token

	resp, _ := doJSON(t, env, http.MethodPost, "/api/users/", map[string]any{
		"uid": "shouldfail", "givenName": "X", "sn": "Y",
		"uidNumber": 60002, "gidNumber": 60002,
	}, viewerToken)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status=%d, want 403", resp.StatusCode)
	}
}
