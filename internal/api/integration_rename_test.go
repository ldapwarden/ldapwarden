//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// TestIntegration_Users_ChangeUID renames a user and asserts the entry moved
// and the references (group membership, another user's manager, and a primary
// group named after the uid) were migrated.
func TestIntegration_Users_ChangeUID(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	user := createTestUser(t, env, token)
	group := createTestGroup(t, env, token)
	suffix := uniqueSuffix(t)
	newUID := "renamed-" + suffix
	newDN := "uid=" + newUID + ",ou=People,dc=example,dc=org"
	t.Cleanup(func() { _, _ = doJSON(t, env, http.MethodDelete, userPath(newDN), nil, token) })

	// user joins the group
	resp, body := doJSON(t, env, http.MethodPost, groupPath(group.DN)+"/members",
		map[string]string{"memberUid": user.UID}, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add member: status=%d body=%s", resp.StatusCode, body)
	}

	// a second user whose manager points at the first user's DN
	manageree := createTestUser(t, env, token)
	resp, body = doJSON(t, env, http.MethodPut, userPath(manageree.DN),
		map[string]any{"manager": user.DN}, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set manager: status=%d body=%s", resp.StatusCode, body)
	}

	// a primary group named after the uid
	resp, body = doJSON(t, env, http.MethodPost, "/api/groups/",
		map[string]any{"cn": user.UID, "gidNumber": testIDNumber(suffix) + 1}, token)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create primary group: status=%d body=%s", resp.StatusCode, body)
	}
	primaryNewDN := "cn=" + newUID + ",ou=Groups,dc=example,dc=org"
	t.Cleanup(func() {
		_, _ = doJSON(t, env, http.MethodDelete, groupPath(primaryNewDN), nil, token)
		_, _ = doJSON(t, env, http.MethodDelete, groupPath("cn="+user.UID+",ou=Groups,dc=example,dc=org"), nil, token)
	})

	// rename
	resp, body = doJSON(t, env, http.MethodPost, userPath(user.DN)+"/change-uid",
		map[string]any{"newUid": newUID}, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("change-uid: status=%d body=%s", resp.StatusCode, body)
	}
	var renameResp struct {
		User     ldap.User `json:"user"`
		Warnings []string  `json:"warnings"`
	}
	if err := json.Unmarshal(body, &renameResp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, body)
	}
	if renameResp.User.UID != newUID {
		t.Errorf("renamed user uid=%q, want %q", renameResp.User.UID, newUID)
	}
	if len(renameResp.Warnings) != 0 {
		t.Errorf("unexpected migration warnings: %v", renameResp.Warnings)
	}

	// old DN gone, new DN present
	if resp, _ := doJSON(t, env, http.MethodGet, userPath(user.DN), nil, token); resp.StatusCode != http.StatusNotFound {
		t.Errorf("old DN still resolves: status=%d", resp.StatusCode)
	}
	if resp, _ := doJSON(t, env, http.MethodGet, userPath(newDN), nil, token); resp.StatusCode != http.StatusOK {
		t.Errorf("new DN missing: status=%d", resp.StatusCode)
	}

	// group membership migrated old->new uid
	_, body = doJSON(t, env, http.MethodGet, groupPath(group.DN), nil, token)
	var g ldap.Group
	_ = json.Unmarshal(body, &g)
	if !contains(g.MemberUIDs, newUID) || contains(g.MemberUIDs, user.UID) {
		t.Errorf("group memberUid not migrated: %v", g.MemberUIDs)
	}

	// manager reference migrated to the new DN
	_, body = doJSON(t, env, http.MethodGet, userPath(manageree.DN), nil, token)
	var m ldap.User
	_ = json.Unmarshal(body, &m)
	if m.Manager != newDN {
		t.Errorf("manager=%q, want %q", m.Manager, newDN)
	}

	// primary group renamed
	if resp, _ := doJSON(t, env, http.MethodGet, groupPath(primaryNewDN), nil, token); resp.StatusCode != http.StatusOK {
		t.Errorf("primary group not renamed to %q", primaryNewDN)
	}
}

// TestIntegration_Users_ChangeUID_Rejections covers the guard rails.
func TestIntegration_Users_ChangeUID_Rejections(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	a := createTestUser(t, env, token)
	b := createTestUser(t, env, token)

	// rename to an already-used uid -> 409
	resp, body := doJSON(t, env, http.MethodPost, userPath(a.DN)+"/change-uid",
		map[string]any{"newUid": b.UID}, token)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("rename to existing uid: status=%d body=%s, want 409", resp.StatusCode, body)
	}

	// invalid uid charset -> 400
	resp, body = doJSON(t, env, http.MethodPost, userPath(a.DN)+"/change-uid",
		map[string]any{"newUid": "bad uid"}, token)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("invalid uid: status=%d body=%s, want 400", resp.StatusCode, body)
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
