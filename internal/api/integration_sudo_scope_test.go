//go:build integration

package api

import (
	"net/http"
	"testing"
)

// TestIntegration_UserSudoRoles_AcceptsAUserDN pins the route's scope.
//
// /users/{dn}/sudo-roles resolved its {dn} against the *sudoers* base, and
// resolveDN requires the DN to sit under the base it is given — so every
// well-formed request was answered 400 and the user page's Sudo tab could never
// load. Only exercising the endpoint catches it; the client function underneath
// was always fine.
func TestIntegration_UserSudoRoles_AcceptsAUserDN(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	dn := "uid=jdoe," + env.LDAP.UserBaseDN()
	resp, body := doJSON(t, env, http.MethodGet, userPath(dn)+"/sudo-roles", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s — a user DN must resolve on a route mounted under /users",
			resp.StatusCode, body)
	}
}

// TestIntegration_GroupSudoRoles_AcceptsAGroupDN is the same defect on the
// group route, which shared the mistake.
func TestIntegration_GroupSudoRoles_AcceptsAGroupDN(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	dn := "cn=developers," + env.LDAP.GroupBaseDN()
	resp, body := doJSON(t, env, http.MethodGet, groupPath(dn)+"/sudo-roles", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s — a group DN must resolve on a route mounted under /groups",
			resp.StatusCode, body)
	}
}
