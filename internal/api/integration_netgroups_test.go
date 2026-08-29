//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

func netgroupPath(dn string) string { return "/api/netgroups/" + url.QueryEscape(dn) }

// TestIntegration_Netgroups_CRUD covers the HTTP surface, which is what the
// browser will read: the client validates these payloads with a Zod schema, so a
// field that does not survive serialisation is a silently broken UI.
func TestIntegration_Netgroups_CRUD(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	cn := "ldapwarden-api-" + uniqueSuffix(t)
	dn := "cn=" + cn + "," + env.LDAP.NetgroupBaseDN()
	t.Cleanup(func() { _ = env.LDAP.DeleteNetgroup(dn) })

	resp, body := doJSON(t, env, http.MethodPost, "/api/netgroups/", map[string]any{
		"cn":          cn,
		"description": "created over HTTP",
		"triples": []map[string]string{
			{"user": "jdoe"},
			{"host": "web01"},
		},
		"members": []string{"ops"},
	}, token)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: status=%d body=%s", resp.StatusCode, body)
	}

	var created ldap.Netgroup
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("unmarshal create: %v (body=%s)", err, body)
	}
	if len(created.Triples) != 2 {
		t.Errorf("created with %d triples, want 2 (body=%s)", len(created.Triples), body)
	}

	// The triples have to survive JSON under the names the UI reads.
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	triples, ok := raw["triples"].([]any)
	if !ok || len(triples) != 2 {
		t.Fatalf("triples missing from the JSON: %s", body)
	}
	first, ok := triples[0].(map[string]any)
	if !ok {
		t.Fatalf("a triple is not an object: %s", body)
	}
	for _, field := range []string{"host", "user", "domain"} {
		if _, present := first[field]; !present {
			t.Errorf("triple has no %q field: %v", field, first)
		}
	}

	// Read back.
	resp, body = doJSON(t, env, http.MethodGet, netgroupPath(dn), nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: status=%d body=%s", resp.StatusCode, body)
	}

	// Update.
	resp, body = doJSON(t, env, http.MethodPut, netgroupPath(dn), map[string]any{
		"triples": []map[string]string{{"user": "alice"}},
	}, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: status=%d body=%s", resp.StatusCode, body)
	}
	var updated ldap.Netgroup
	if err := json.Unmarshal(body, &updated); err != nil {
		t.Fatalf("unmarshal update: %v", err)
	}
	if len(updated.Triples) != 1 || !updated.Triples[0].NamesUser("alice") {
		t.Errorf("after update, triples = %v", updated.Triples)
	}

	// Expansion follows the nesting into the seeded ops netgroup.
	resp, body = doJSON(t, env, http.MethodGet, netgroupPath(dn)+"/expand", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expand: status=%d body=%s", resp.StatusCode, body)
	}
	var expansion ldap.NetgroupExpansion
	if err := json.Unmarshal(body, &expansion); err != nil {
		t.Fatalf("unmarshal expand: %v", err)
	}
	names := map[string]bool{}
	for _, member := range expansion.Users {
		names[member.Name] = true
	}
	for _, want := range []string{"alice", "admin", "jdoe"} {
		if !names[want] {
			t.Errorf("expansion missing %q; got %v", want, expansion.Users)
		}
	}
	// Every seeded triple leaves the host slot open, so this matches every host —
	// the case a bare list would report as none.
	if !expansion.AnyHost() {
		t.Errorf("AnyHost() is false; openHost=%v", expansion.OpenHost)
	}

	resp, _ = doJSON(t, env, http.MethodDelete, netgroupPath(dn), nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete: status=%d", resp.StatusCode)
	}
	resp, _ = doJSON(t, env, http.MethodGet, netgroupPath(dn), nil, token)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("after delete, get returned %d, want 404", resp.StatusCode)
	}
}

// TestIntegration_Netgroups_RejectsWideOpenTriple: "(,,)" is legal in the schema
// and means every user on every host. The directory accepts it; the API does not.
func TestIntegration_Netgroups_RejectsWideOpenTriple(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	cn := "ldapwarden-api-wide-" + uniqueSuffix(t)
	resp, body := doJSON(t, env, http.MethodPost, "/api/netgroups/", map[string]any{
		"cn":      cn,
		"triples": []map[string]string{{}},
	}, token)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status=%d body=%s, want 400", resp.StatusCode, body)
		_ = env.LDAP.DeleteNetgroup("cn=" + cn + "," + env.LDAP.NetgroupBaseDN())
	}
}

// TestIntegration_Netgroups_ReadOnlyCannotWrite pins the permission decision:
// netgroups are governed by groups:*, so the read-only role can list them and
// cannot create one.
func TestIntegration_Netgroups_ReadOnlyCannotWrite(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "viewer", "viewer123").Token

	resp, body := doJSON(t, env, http.MethodGet, "/api/netgroups/", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("list as viewer: status=%d body=%s, want 200", resp.StatusCode, body)
	}

	resp, _ = doJSON(t, env, http.MethodPost, "/api/netgroups/", map[string]any{
		"cn":      "ldapwarden-api-denied",
		"triples": []map[string]string{{"user": "jdoe"}},
	}, token)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("create as viewer: status=%d, want 403", resp.StatusCode)
	}
}

// TestIntegration_Netgroups_DeleteWarnsAboutDanglingReferences: sudo names a
// netgroup by "+name" and nothing in the directory ties that string to the
// entry, so a delete leaves those rules matching nobody. Silently, unless the
// response says so.
func TestIntegration_Netgroups_DeleteWarnsAboutDanglingReferences(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	suffix := uniqueSuffix(t)
	ngCN := "ldapwarden-dangling-" + suffix
	ngDN := "cn=" + ngCN + "," + env.LDAP.NetgroupBaseDN()
	t.Cleanup(func() { _ = env.LDAP.DeleteNetgroup(ngDN) })

	if _, err := env.LDAP.CreateNetgroup(ldap.CreateNetgroupRequest{
		CN:      ngCN,
		Triples: []ldap.NetgroupTriple{{User: "jdoe"}},
	}); err != nil {
		t.Fatalf("create netgroup: %v", err)
	}

	// A sudo role naming it, and a netgroup nesting it.
	roleCN := "ldapwarden-dangling-role-" + suffix
	role, err := env.LDAP.CreateSudoRole(ldap.CreateSudoRoleRequest{
		CN: roleCN, SudoUser: []string{"+" + ngCN}, SudoHost: []string{"ALL"},
		SudoCommand: []string{"/bin/true"},
	})
	if err != nil {
		t.Fatalf("create sudo role: %v", err)
	}
	t.Cleanup(func() { _ = env.LDAP.DeleteSudoRole(role.DN) })

	parentCN := "ldapwarden-dangling-parent-" + suffix
	parent, err := env.LDAP.CreateNetgroup(ldap.CreateNetgroupRequest{
		CN: parentCN, Members: []string{ngCN},
	})
	if err != nil {
		t.Fatalf("create parent netgroup: %v", err)
	}
	t.Cleanup(func() { _ = env.LDAP.DeleteNetgroup(parent.DN) })

	resp, body := doJSON(t, env, http.MethodDelete, netgroupPath(ngDN), nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete: status=%d body=%s", resp.StatusCode, body)
	}

	var result struct {
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, body)
	}

	var sawRole, sawParent bool
	for _, w := range result.Warnings {
		if strings.Contains(w, roleCN) {
			sawRole = true
		}
		if strings.Contains(w, parentCN) {
			sawParent = true
		}
	}
	if !sawRole {
		t.Errorf("no warning about the sudo role still naming +%s: %v", ngCN, result.Warnings)
	}
	if !sawParent {
		t.Errorf("no warning about the netgroup still nesting it: %v", result.Warnings)
	}
}

// TestIntegration_UserSudoRoles_ReportsWhyARoleApplies covers the provenance.
//
// A role reached through a group or a netgroup is edited somewhere other than
// the user's page, and "+ops" reads like a username to anything that does not
// know what ops is. Without `via` the three cases are indistinguishable.
func TestIntegration_UserSudoRoles_ReportsWhyARoleApplies(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	// jdoe is seeded into the ops netgroup, which ops-oncall nests.
	dn := "uid=jdoe," + env.LDAP.UserBaseDN()

	roleCN := "ldapwarden-via-ng-" + uniqueSuffix(t)
	role, err := env.LDAP.CreateSudoRole(ldap.CreateSudoRoleRequest{
		CN: roleCN, SudoUser: []string{"+ops-oncall"}, SudoHost: []string{"ALL"},
		SudoCommand: []string{"/bin/true"},
	})
	if err != nil {
		t.Fatalf("create sudo role: %v", err)
	}
	t.Cleanup(func() { _ = env.LDAP.DeleteSudoRole(role.DN) })

	resp, body := doJSON(t, env, http.MethodGet, userPath(dn)+"/sudo-roles", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}

	var page struct {
		Data []struct {
			CN  string   `json:"cn"`
			Via []string `json:"via"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, body)
	}

	var found bool
	for _, grant := range page.Data {
		if grant.CN != roleCN {
			continue
		}
		found = true
		// Reached only through nesting: ops-oncall absorbs ops, which names jdoe.
		if !slices.Contains(grant.Via, "+ops-oncall") {
			t.Errorf("via = %v, want it to name +ops-oncall", grant.Via)
		}
		if slices.Contains(grant.Via, "jdoe") {
			t.Errorf("via = %v claims a direct grant, but the role names no uid", grant.Via)
		}
	}
	if !found {
		t.Errorf("the role granted through a netgroup is missing; got %v", page.Data)
	}
}

// TestIntegration_User_Netgroups covers GET /users/{dn}/netgroups over HTTP.
//
// Both halves of the answer — every netgroup that reaches the user, and which of
// them name them directly — come out of one graph walk. The distinction is what
// the page acts on: an indirect membership still confers whatever "+parent"
// grants, but the parent is not the entry to edit to take it away, so reporting
// the two alike would send an operator to the wrong screen.
func TestIntegration_User_Netgroups(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	user := createTestUser(t, env, token)
	t.Cleanup(func() {
		_, _ = doJSON(t, env, http.MethodDelete, userPath(user.DN), nil, token)
	})

	suffix := uniqueSuffix(t)
	child := "ldapwarden-child-" + suffix
	parent := "ldapwarden-parent-" + suffix
	unrelated := "ldapwarden-other-" + suffix
	t.Cleanup(func() {
		for _, cn := range []string{child, parent, unrelated} {
			_ = env.LDAP.DeleteNetgroup("cn=" + cn + "," + env.LDAP.NetgroupBaseDN())
		}
	})

	for _, req := range []ldap.CreateNetgroupRequest{
		{CN: child, Triples: []ldap.NetgroupTriple{{User: user.UID}}},
		{CN: parent, Members: []string{child}},
		// Names the uid in the *host* slot, which is not membership.
		{CN: unrelated, Triples: []ldap.NetgroupTriple{{Host: user.UID}}},
	} {
		if _, err := env.LDAP.CreateNetgroup(req); err != nil {
			t.Fatalf("create netgroup %q: %v", req.CN, err)
		}
	}

	resp, body := doJSON(t, env, http.MethodGet, userPath(user.DN)+"/netgroups", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get user netgroups: status=%d body=%s", resp.StatusCode, body)
	}

	// Read as raw JSON: the browser validates this with a Zod schema, so the
	// field names are part of the contract, not an implementation detail.
	var page struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, body)
	}

	direct := map[string]bool{}
	for _, item := range page.Data {
		cn, ok := item["cn"].(string)
		if !ok {
			t.Fatalf("an entry has no %q field: %v", "cn", item)
		}
		flag, ok := item["direct"].(bool)
		if !ok {
			t.Fatalf("entry %q has no %q field: %v", cn, "direct", item)
		}
		direct[cn] = flag
	}

	if got, ok := direct[child]; !ok || !got {
		t.Errorf("%q direct = %v (present %v), want true", child, got, ok)
	}
	if got, ok := direct[parent]; !ok || got {
		t.Errorf("%q direct = %v (present %v), want present and false", parent, got, ok)
	}
	if _, ok := direct[unrelated]; ok {
		t.Errorf("%q is listed, but it names the uid in the host slot", unrelated)
	}
}
