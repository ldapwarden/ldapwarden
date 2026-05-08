//go:build integration

package api

import (
	"net/http"
	"testing"
)

// TestIntegration_Sessions_InvalidatedOnUserDelete verifies that an active
// session token belonging to a deleted user is rejected on the next request,
// without waiting for SESSION_TTL to expire.
func TestIntegration_Sessions_InvalidatedOnUserDelete(t *testing.T) {
	env := setupTestServer(t)
	adminToken := loginAs(t, env, "admin", "admin123").Token

	user := createTestUser(t, env, adminToken)

	// Login as the freshly-created user. createTestUser sets password "testpass".
	userToken := loginAs(t, env, user.UID, "testpass").Token

	resp, _ := doJSON(t, env, http.MethodGet, "/api/auth/me", nil, userToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/me before delete: status=%d, want 200", resp.StatusCode)
	}

	resp, body := doJSON(t, env, http.MethodDelete, userPath(user.DN), nil, adminToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete user: status=%d body=%s", resp.StatusCode, body)
	}

	resp, _ = doJSON(t, env, http.MethodGet, "/api/auth/me", nil, userToken)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("/me after delete: status=%d, want 401 (session must be invalidated)", resp.StatusCode)
	}
}

// TestIntegration_Sessions_InvalidatedOnAdminDemotion verifies that removing
// a user from the admin group drops their existing session immediately,
// instead of leaving a 24-hour window in which the cached permissions still
// grant admin rights.
func TestIntegration_Sessions_InvalidatedOnAdminDemotion(t *testing.T) {
	env := setupTestServer(t)
	adminToken := loginAs(t, env, "admin", "admin123").Token

	user := createTestUser(t, env, adminToken)
	adminGroupDN := "cn=" + env.Config.App.AdminGroup + "," + env.Config.LDAP.GroupOU + "," + env.Config.LDAP.BaseDN

	// Promote.
	resp, body := doJSON(t, env, http.MethodPost, groupPath(adminGroupDN)+"/members",
		map[string]string{"memberUid": user.UID}, adminToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add to admin group: status=%d body=%s", resp.StatusCode, body)
	}
	// Best-effort cleanup if the demotion below never runs.
	t.Cleanup(func() {
		_, _ = doJSON(t, env, http.MethodDelete, groupPath(adminGroupDN)+"/members",
			map[string]string{"memberUid": user.UID}, adminToken)
	})

	lr := loginAs(t, env, user.UID, "testpass")
	hasWrite := false
	for _, p := range lr.Session.Permissions {
		if p == "users:write" {
			hasWrite = true
			break
		}
	}
	if !hasWrite {
		t.Fatalf("test user not promoted to admin: permissions=%v", lr.Session.Permissions)
	}
	userToken := lr.Token

	resp, _ = doJSON(t, env, http.MethodGet, "/api/auth/me", nil, userToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/me before demotion: status=%d, want 200", resp.StatusCode)
	}

	// Demote — this must invalidate the user's session.
	resp, body = doJSON(t, env, http.MethodDelete, groupPath(adminGroupDN)+"/members",
		map[string]string{"memberUid": user.UID}, adminToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("remove from admin group: status=%d body=%s", resp.StatusCode, body)
	}

	resp, _ = doJSON(t, env, http.MethodGet, "/api/auth/me", nil, userToken)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("/me after demotion: status=%d, want 401 (session must be invalidated)", resp.StatusCode)
	}
}
