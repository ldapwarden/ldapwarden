//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// TestIntegration_Groups_Delete_ReportsLeftovers: a group is named by string
// from sudo and by number from its primary members, and neither follows the
// entry when it is deleted. The delete goes through — removing a group is
// legitimate — but both must come back as warnings, or the operator never
// learns that a rule now grants to nobody and that accounts still carry a
// gidNumber the next group to take it will inherit.
func TestIntegration_Groups_Delete_ReportsLeftovers(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	group := createTestGroup(t, env, token)
	role := createSudoRole(t, env, token, "ldapwarden-delete-"+group.CN, []string{"%" + group.CN})

	// An account whose primary group is this one: joined by number, not listed
	// in memberUid.
	suffix := uniqueSuffix(t)
	uid := "primary-" + suffix
	resp, body := doJSON(t, env, http.MethodPost, "/api/users/", map[string]any{
		"uid":       uid,
		"givenName": "Primary",
		"sn":        "Member-" + suffix,
		"uidNumber": testIDNumber(suffix),
		"gidNumber": group.GIDNumber,
		"mail":      uid + "@test.example",
		"password":  "testpass",
	}, token)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create primary member: status=%d body=%s", resp.StatusCode, body)
	}
	var member struct {
		DN string `json:"dn"`
	}
	if err := json.Unmarshal(body, &member); err != nil {
		t.Fatalf("unmarshal user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = doJSON(t, env, http.MethodDelete, userPath(member.DN), nil, token)
	})

	resp, body = doJSON(t, env, http.MethodDelete, groupPath(group.DN), nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete: status=%d body=%s", resp.StatusCode, body)
	}
	var result struct {
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, body)
	}

	joined := strings.Join(result.Warnings, "\n")
	if !strings.Contains(joined, role.CN) || !strings.Contains(joined, "%"+group.CN) {
		t.Errorf("warnings do not name sudo role %q granting to %%%s: %v", role.CN, group.CN, result.Warnings)
	}
	if !strings.Contains(joined, uid) {
		t.Errorf("warnings do not name primary member %q: %v", uid, result.Warnings)
	}

	if _, err := env.LDAP.GetGroupByCN(group.CN); err == nil {
		t.Errorf("group %q still exists after a successful delete", group.CN)
	}
}

// TestIntegration_Groups_Delete_Clean: nothing referencing the group, so the
// answer carries an empty list — not null, which the client would have to
// special-case.
func TestIntegration_Groups_Delete_Clean(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	group := createTestGroup(t, env, token)

	resp, body := doJSON(t, env, http.MethodDelete, groupPath(group.DN), nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete: status=%d body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"warnings":[]`) {
		t.Errorf("want an empty warnings array on a clean delete, got %s", body)
	}
}

// TestIntegration_Groups_Delete_RefusesAdminGroup: administrator access is
// decided by comparing group names against LDAPWARDEN_ADMIN_GROUP, so deleting
// that group takes admin away from everyone, whoever did it included. Refused,
// as a rename of it is.
func TestIntegration_Groups_Delete_RefusesAdminGroup(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	adminCN := env.Config.App.AdminGroup
	adminGroup, err := env.LDAP.GetGroupByCN(adminCN)
	if err != nil {
		t.Skipf("admin group %q not in this directory: %v", adminCN, err)
	}
	// If the guard ever regresses, this test deletes the admin group for real
	// and every later run starts from a directory where nobody is an
	// administrator. Put it back before failing.
	t.Cleanup(func() {
		if _, err := env.LDAP.GetGroupByCN(adminCN); err == nil {
			return
		}
		if _, err := env.LDAP.CreateGroup(ldap.CreateGroupRequest{
			CN:          adminGroup.CN,
			GIDNumber:   adminGroup.GIDNumber,
			Description: adminGroup.Description,
			MemberUIDs:  adminGroup.MemberUIDs,
		}); err != nil {
			t.Errorf("could not restore the admin group: %v", err)
		}
	})

	resp, body := doJSON(t, env, http.MethodDelete, groupPath(adminGroup.DN), nil, token)
	if resp.StatusCode == http.StatusOK {
		t.Fatal("deleting the administrator group succeeded; every admin would have lost access")
	}
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status=%d, want 409 (body=%s)", resp.StatusCode, body)
	}
	if _, err := env.LDAP.GetGroupByCN(adminCN); err != nil {
		t.Errorf("the admin group was deleted anyway: %v", err)
	}
}
