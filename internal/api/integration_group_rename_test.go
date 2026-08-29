//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// TestIntegration_Groups_Rename covers renaming a group and the one reference
// the directory will not follow.
//
// A POSIX group is joined by number and by username, so a rename leaves its
// members exactly where they were — that is the assertion that stops a future
// change from "migrating" memberships that never needed migrating. Sudo is the
// exception: it grants to "%cn", a bare string nothing tracks, so a rule left
// alone would go on naming a group that no longer exists.
func TestIntegration_Groups_Rename(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	group := createTestGroup(t, env, token)
	member := createTestUser(t, env, token)
	newCN := group.CN + "-renamed"
	t.Cleanup(func() {
		for _, cn := range []string{group.CN, newCN} {
			_, _ = doJSON(t, env, http.MethodDelete,
				groupPath("cn="+cn+","+env.LDAP.GroupBaseDN()), nil, token)
		}
	})

	resp, body := doJSON(t, env, http.MethodPost, groupPath(group.DN)+"/members",
		map[string]string{"memberUid": member.UID}, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add member: status=%d body=%s", resp.StatusCode, body)
	}

	role := createSudoRole(t, env, token, "ldapwarden-rename-"+group.CN, []string{"%" + group.CN})

	resp, body = doJSON(t, env, http.MethodPost, groupPath(group.DN)+"/change-cn",
		map[string]string{"newCn": newCN}, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rename: status=%d body=%s", resp.StatusCode, body)
	}

	var result struct {
		Group    ldap.Group `json:"group"`
		Warnings []string   `json:"warnings"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, body)
	}
	if result.Group.CN != newCN {
		t.Errorf("cn = %q, want %q", result.Group.CN, newCN)
	}
	// The DN follows the cn, since it is the RDN.
	if want := "cn=" + newCN + "," + env.LDAP.GroupBaseDN(); result.Group.DN != want {
		t.Errorf("dn = %q, want %q", result.Group.DN, want)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("warnings on a clean rename: %v", result.Warnings)
	}

	// The old name must be gone, not left behind as a second cn value.
	if _, err := env.LDAP.GetGroupByCN(group.CN); err == nil {
		t.Errorf("the group is still reachable under its old name %q", group.CN)
	}

	// Membership is by username, so it did not move — and must not have been
	// disturbed by the rename.
	renamed, err := env.LDAP.GetGroupByCN(newCN)
	if err != nil {
		t.Fatalf("read renamed group: %v", err)
	}
	if !slices.Contains(renamed.MemberUIDs, member.UID) {
		t.Errorf("members = %v, want %q still there", renamed.MemberUIDs, member.UID)
	}
	if renamed.GIDNumber != group.GIDNumber {
		t.Errorf("gidNumber = %d, want %d unchanged", renamed.GIDNumber, group.GIDNumber)
	}

	// The sudo grant had to be rewritten, or it now names nobody.
	roles, err := env.LDAP.GetGroupSudoRoles(newCN)
	if err != nil {
		t.Fatalf("read sudo roles for the new name: %v", err)
	}
	found := false
	for _, r := range roles {
		if r.CN == role.CN {
			found = true
		}
	}
	if !found {
		t.Errorf("no sudo rule grants to %%%s after the rename", newCN)
	}
	if stale, err := env.LDAP.GetGroupSudoRoles(group.CN); err != nil {
		t.Fatalf("read sudo roles for the old name: %v", err)
	} else if len(stale) != 0 {
		t.Errorf("%d sudo rule(s) still grant to %%%s, which no longer exists", len(stale), group.CN)
	}
}

// TestIntegration_Groups_Rename_RefusesAdminGroup: administrator access is
// decided by comparing group names against LDAPWARDEN_ADMIN_GROUP, so renaming
// that group takes admin away from everyone — including whoever did it, who
// could then not undo it from here. Refused rather than warned about, like a
// user renaming themselves.
func TestIntegration_Groups_Rename_RefusesAdminGroup(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	adminCN := env.Config.App.AdminGroup
	adminGroup, err := env.LDAP.GetGroupByCN(adminCN)
	if err != nil {
		t.Skipf("admin group %q not in this directory: %v", adminCN, err)
	}

	renamedCN := adminCN + "-renamed"
	// If the guard ever regresses, this test renames the admin group for real and
	// every later run starts from a directory where nobody is an administrator.
	// Put it back before failing.
	t.Cleanup(func() {
		if _, err := env.LDAP.GetGroupByCN(adminCN); err == nil {
			return
		}
		renamed, err := env.LDAP.GetGroupByCN(renamedCN)
		if err != nil {
			return
		}
		if _, _, err := env.LDAP.RenameGroup(renamed.DN, renamedCN, adminCN); err != nil {
			t.Errorf("could not restore the admin group name: %v", err)
		}
	})

	resp, body := doJSON(t, env, http.MethodPost, groupPath(adminGroup.DN)+"/change-cn",
		map[string]string{"newCn": renamedCN}, token)
	if resp.StatusCode == http.StatusOK {
		t.Fatal("renaming the administrator group succeeded; every admin would have lost access")
	}
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status=%d, want 409 (body=%s)", resp.StatusCode, body)
	}

	// Still there, under its own name.
	if _, err := env.LDAP.GetGroupByCN(adminCN); err != nil {
		t.Errorf("the admin group was renamed anyway: %v", err)
	}
}

func TestIntegration_Groups_Rename_RejectsDuplicateAndBadNames(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	group := createTestGroup(t, env, token)
	other := createTestGroup(t, env, token)

	for _, tc := range []struct {
		name  string
		newCN string
		want  int
	}{
		{name: "taken", newCN: other.CN, want: http.StatusConflict},
		{name: "unchanged", newCN: group.CN, want: http.StatusBadRequest},
		{name: "invalid rdn", newCN: "bad name with spaces", want: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := doJSON(t, env, http.MethodPost, groupPath(group.DN)+"/change-cn",
				map[string]string{"newCn": tc.newCN}, token)
			if resp.StatusCode != tc.want {
				t.Errorf("status=%d, want %d (body=%s)", resp.StatusCode, tc.want, body)
			}
		})
	}

	// None of the refusals may have renamed anything.
	if _, err := env.LDAP.GetGroupByCN(group.CN); err != nil {
		t.Errorf("the group was renamed by a request that should have been refused: %v", err)
	}
}
