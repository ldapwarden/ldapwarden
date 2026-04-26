//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// TestIntegration_Groups_CRUDLifecycle covers create → get → update → delete.
func TestIntegration_Groups_CRUDLifecycle(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	group := createTestGroup(t, env, token)

	resp, body := doJSON(t, env, http.MethodGet, groupPath(group.DN), nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get group: status=%d body=%s", resp.StatusCode, body)
	}
	var got ldap.Group
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.CN != group.CN {
		t.Errorf("CN=%q, want %q", got.CN, group.CN)
	}

	// Update description.
	newDesc := "updated " + group.CN
	resp, body = doJSON(t, env, http.MethodPut, groupPath(group.DN),
		map[string]any{"description": newDesc}, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update group: status=%d body=%s", resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal updated: %v", err)
	}
	if got.Description != newDesc {
		t.Errorf("Description=%q, want %q", got.Description, newDesc)
	}

	// Delete.
	resp, _ = doJSON(t, env, http.MethodDelete, groupPath(group.DN), nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete: status=%d", resp.StatusCode)
	}
	resp, _ = doJSON(t, env, http.MethodGet, groupPath(group.DN), nil, token)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("get after delete: status=%d, want 404", resp.StatusCode)
	}
}

// TestIntegration_Groups_MemberLifecycle creates a user and a group, then adds
// and removes the user as a member.
func TestIntegration_Groups_MemberLifecycle(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	user := createTestUser(t, env, token)
	group := createTestGroup(t, env, token)

	// Add member.
	resp, body := doJSON(t, env, http.MethodPost, groupPath(group.DN)+"/members",
		map[string]string{"memberUid": user.UID}, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add member: status=%d body=%s", resp.StatusCode, body)
	}

	// Verify presence (fresh struct: omitempty fields don't get reset by Unmarshal).
	_, body = doJSON(t, env, http.MethodGet, groupPath(group.DN), nil, token)
	var afterAdd ldap.Group
	if err := json.Unmarshal(body, &afterAdd); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !slices.Contains(afterAdd.MemberUIDs, user.UID) {
		t.Errorf("MemberUIDs=%v, missing %q", afterAdd.MemberUIDs, user.UID)
	}

	// Remove.
	resp, _ = doJSON(t, env, http.MethodDelete, groupPath(group.DN)+"/members",
		map[string]string{"memberUid": user.UID}, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("remove member: status=%d", resp.StatusCode)
	}

	// Verify absence.
	_, body = doJSON(t, env, http.MethodGet, groupPath(group.DN), nil, token)
	var afterRemove ldap.Group
	if err := json.Unmarshal(body, &afterRemove); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if slices.Contains(afterRemove.MemberUIDs, user.UID) {
		t.Errorf("MemberUIDs=%v, %q still present", afterRemove.MemberUIDs, user.UID)
	}
}

func TestIntegration_Groups_CreateRequiresFields(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	// Missing cn.
	resp, _ := doJSON(t, env, http.MethodPost, "/api/groups/",
		map[string]any{"gidNumber": 60001}, token)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing cn: status=%d, want 400", resp.StatusCode)
	}

	// Missing gidNumber.
	resp, _ = doJSON(t, env, http.MethodPost, "/api/groups/",
		map[string]any{"cn": "noid"}, token)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing gidNumber: status=%d, want 400", resp.StatusCode)
	}
}

func TestIntegration_Groups_GetMissing(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	resp, _ := doJSON(t, env, http.MethodGet,
		groupPath("cn=nosuch,ou=Groups,dc=example,dc=org"), nil, token)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status=%d, want 404", resp.StatusCode)
	}
}

func TestIntegration_Groups_AddMember_UnknownUserStillSucceeds(t *testing.T) {
	// Documents current behaviour: posixGroup.memberUid is a free-form string,
	// LDAP doesn't enforce that the UID corresponds to an existing user.
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	group := createTestGroup(t, env, token)

	resp, _ := doJSON(t, env, http.MethodPost, groupPath(group.DN)+"/members",
		map[string]string{"memberUid": "ghost-user-does-not-exist"}, token)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status=%d, want 200 (no referential integrity)", resp.StatusCode)
	}
}
