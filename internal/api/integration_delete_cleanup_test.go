//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// TestIntegration_Users_DeleteCleansUpReferences pins that deleting a user takes
// their uid out of the places that stored it.
//
// Deleting only the entry used to leave memberUid and sudoUser behind, because
// neither holds a DN the directory could follow. That is not cosmetic: recreating
// an account with the same uid — routine when a username is recycled — silently
// handed it the previous holder's groups and sudo grants.
func TestIntegration_Users_DeleteCleansUpReferences(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	user := createTestUser(t, env, token)
	group := createTestGroup(t, env, token)

	// The user is a member of a group...
	if resp, body := doJSON(t, env, http.MethodPost, groupPath(group.DN)+"/members",
		map[string]string{"memberUid": user.UID}, token); resp.StatusCode != http.StatusOK {
		t.Fatalf("add member: status=%d body=%s", resp.StatusCode, body)
	}

	// ...named outright by one sudo role, and covered by another only through
	// "ALL", which must survive untouched.
	suffix := uniqueSuffix(t)
	named := createSudoRole(t, env, token, "sudo-named-"+suffix, []string{user.UID, "root"})
	viaAll := createSudoRole(t, env, token, "sudo-all-"+suffix, []string{"ALL"})
	onlyMe := createSudoRole(t, env, token, "sudo-only-"+suffix, []string{user.UID})

	// ...and is another user's manager.
	subordinate := createTestUser(t, env, token)
	if resp, body := doJSON(t, env, http.MethodPut, userPath(subordinate.DN),
		map[string]any{"manager": user.DN}, token); resp.StatusCode != http.StatusOK {
		t.Fatalf("set manager: status=%d body=%s", resp.StatusCode, body)
	}

	resp, body := doJSON(t, env, http.MethodDelete, userPath(user.DN), nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete user: status=%d body=%s", resp.StatusCode, body)
	}
	var deleted struct {
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(body, &deleted); err != nil {
		t.Fatalf("unmarshal delete response: %v", err)
	}

	// The group must no longer list the uid.
	for _, uid := range groupMembers(t, env, token, group.DN) {
		if uid == user.UID {
			t.Errorf("group %q still lists deleted member %q", group.CN, user.UID)
		}
	}

	// The role that named the user drops it and keeps its other entries.
	after := getSudoRole(t, env, token, named.DN)
	if containsString(after.SudoUser, user.UID) {
		t.Errorf("sudo role %q still grants to deleted user %q: %v", named.CN, user.UID, after.SudoUser)
	}
	if !containsString(after.SudoUser, "root") {
		t.Errorf("sudo role %q lost an unrelated sudoUser: %v", named.CN, after.SudoUser)
	}

	// "ALL" is not a reference to this user and must be left alone.
	if all := getSudoRole(t, env, token, viaAll.DN); !containsString(all.SudoUser, "ALL") {
		t.Errorf("sudo role %q lost its ALL entry: %v", viaAll.CN, all.SudoUser)
	}

	// A role that named only this user is emptied but kept — and reported, so an
	// emptied rule does not sit there unnoticed.
	if only := getSudoRole(t, env, token, onlyMe.DN); containsString(only.SudoUser, user.UID) {
		t.Errorf("sudo role %q still grants to deleted user %q", onlyMe.CN, user.UID)
	}
	if len(deleted.Warnings) == 0 {
		t.Errorf("emptying sudo role %q was not reported: warnings=%v", onlyMe.CN, deleted.Warnings)
	}

	// The dangling manager reference is cleared.
	if mgr := getUser(t, env, token, subordinate.DN).Manager; mgr != "" {
		t.Errorf("manager still points at the deleted user: %q", mgr)
	}
}

// TestIntegration_Users_DeleteReportsNoWarningsWhenClean is the other half: a
// user with nothing referencing them deletes silently, so the warning channel
// stays meaningful.
func TestIntegration_Users_DeleteReportsNoWarningsWhenClean(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token
	user := createTestUser(t, env, token)

	resp, body := doJSON(t, env, http.MethodDelete, userPath(user.DN), nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete user: status=%d body=%s", resp.StatusCode, body)
	}
	var deleted struct {
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(body, &deleted); err != nil {
		t.Fatalf("unmarshal delete response: %v", err)
	}
	if len(deleted.Warnings) != 0 {
		t.Errorf("unreferenced user produced warnings: %v", deleted.Warnings)
	}
}

// createSudoRole creates a sudo role with the given sudoUser values and registers
// its cleanup.
func createSudoRole(t *testing.T, env *testEnv, token, cn string, users []string) *ldap.SudoRole {
	t.Helper()

	resp, body := doJSON(t, env, http.MethodPost, "/api/sudo-roles/", map[string]any{
		"cn":          cn,
		"sudoUser":    users,
		"sudoHost":    []string{"ALL"},
		"sudoCommand": []string{"/usr/bin/true"},
	}, token)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create sudo role %q: status=%d body=%s", cn, resp.StatusCode, body)
	}
	var role ldap.SudoRole
	if err := json.Unmarshal(body, &role); err != nil {
		t.Fatalf("unmarshal sudo role: %v", err)
	}
	t.Cleanup(func() {
		_, _ = doJSON(t, env, http.MethodDelete, sudoPath(role.DN), nil, token)
	})
	return &role
}

func getSudoRole(t *testing.T, env *testEnv, token, dn string) ldap.SudoRole {
	t.Helper()
	resp, body := doJSON(t, env, http.MethodGet, sudoPath(dn), nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get sudo role: status=%d body=%s", resp.StatusCode, body)
	}
	var role ldap.SudoRole
	if err := json.Unmarshal(body, &role); err != nil {
		t.Fatalf("unmarshal sudo role: %v", err)
	}
	return role
}

// groupMembers reads a group's membership through the resolver endpoint.
func groupMembers(t *testing.T, env *testEnv, token, dn string) []string {
	t.Helper()
	resp, body := doJSON(t, env, http.MethodGet, groupPath(dn)+"/members", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get group members: status=%d body=%s", resp.StatusCode, body)
	}
	var payload struct {
		Data []ldap.GroupMember `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal members: %v", err)
	}
	uids := make([]string, 0, len(payload.Data))
	for _, m := range payload.Data {
		uids = append(uids, m.UID)
	}
	return uids
}

func containsString(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}
