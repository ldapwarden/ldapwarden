//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// TestIntegration_CreateUser_RefusesATakenUIDNumber is the regression for a bug
// that was reachable from every creation path: nothing ever checked whether a
// POSIX number was already in use, and OpenLDAP enforces no uniqueness on
// uidNumber unless the `unique` overlay is configured — which the bundled stack
// does not do. Two accounts on one number are one user to the kernel: same
// files, same access, and nothing afterwards can tell them apart.
func TestIntegration_CreateUser_RefusesATakenUIDNumber(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	first := createTestUser(t, env, token)
	uid := "zz-dup-" + uniqueSuffix(t)
	t.Cleanup(func() {
		_, _ = doJSON(t, env, http.MethodDelete, userPath(first.DN), nil, token)
		_, _ = doJSON(t, env, http.MethodDelete,
			userPath("uid="+uid+","+env.LDAP.UserBaseDN()), nil, token)
	})

	resp, body := doJSON(t, env, http.MethodPost, "/api/users/", map[string]any{
		"uid": uid, "givenName": "Dup", "sn": "Probe",
		"uidNumber": first.UIDNumber, "gidNumber": first.GIDNumber,
	}, token)

	if resp.StatusCode == http.StatusCreated {
		t.Fatalf("a second account was created on uid number %d", first.UIDNumber)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status=%d, want 409 (body=%s)", resp.StatusCode, body)
	}
	// Naming the holder is how the operator picks another number.
	if !strings.Contains(string(body), first.UID) {
		t.Errorf("the message does not name the account already holding it: %s", body)
	}

	holders, err := usersWithUIDNumber(env, first.UIDNumber)
	if err != nil {
		t.Fatalf("count holders: %v", err)
	}
	if holders != 1 {
		t.Errorf("%d accounts carry uid number %d", holders, first.UIDNumber)
	}
}

// Zero means "allocate one", settled at write time rather than read from a form
// that loaded minutes ago — which is the window two simultaneous creations both
// used to fall through.
func TestIntegration_CreateUser_AllocatesWhenNoNumberGiven(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	uid := "zz-alloc-" + uniqueSuffix(t)
	t.Cleanup(func() {
		_, _ = doJSON(t, env, http.MethodDelete,
			userPath("uid="+uid+","+env.LDAP.UserBaseDN()), nil, token)
	})

	resp, body := doJSON(t, env, http.MethodPost, "/api/users/", map[string]any{
		"uid": uid, "givenName": "Alloc", "sn": "Probe", "gidNumber": env.LDAP.MinGID(),
	}, token)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: status=%d body=%s", resp.StatusCode, body)
	}

	var created ldap.User
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if created.UIDNumber < env.LDAP.MinUID() {
		t.Errorf("uidNumber = %d, want at least MinUID %d", created.UIDNumber, env.LDAP.MinUID())
	}
	holders, err := usersWithUIDNumber(env, created.UIDNumber)
	if err != nil {
		t.Fatalf("count holders: %v", err)
	}
	if holders != 1 {
		t.Errorf("the allocated number %d is shared by %d accounts", created.UIDNumber, holders)
	}
}

// A file listing one number twice: the directory cannot know about a row that
// has not been written yet, so each row looked fine on its own and both were
// created.
func TestIntegration_ImportUsers_RefusesADuplicateWithinTheFile(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	suffix := uniqueSuffix(t)
	a, b := "zz-imp-a-"+suffix, "zz-imp-b-"+suffix
	n := testIDNumber(suffix)
	t.Cleanup(func() {
		for _, uid := range []string{a, b} {
			_, _ = doJSON(t, env, http.MethodDelete,
				userPath("uid="+uid+","+env.LDAP.UserBaseDN()), nil, token)
		}
	})

	resp, body := doJSON(t, env, http.MethodPost, "/api/users/import", map[string]any{
		"rows": []map[string]any{
			{"uid": a, "givenName": "I", "sn": "A", "uidNumber": n, "gidNumber": n},
			{"uid": b, "givenName": "I", "sn": "B", "uidNumber": n, "gidNumber": n},
		},
	}, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("import: status=%d body=%s", resp.StatusCode, body)
	}

	var result importResultBody
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Created != 1 || result.Failed != 1 {
		t.Fatalf("created=%d failed=%d, want 1/1 (body=%s)", result.Created, result.Failed, body)
	}
	// The message has to point at the other row, or the operator cannot find it
	// in a 500-line file.
	if !strings.Contains(string(body), a) {
		t.Errorf("the failure does not name the row that claimed the number first: %s", body)
	}

	holders, err := usersWithUIDNumber(env, n)
	if err != nil {
		t.Fatalf("count holders: %v", err)
	}
	if holders != 1 {
		t.Errorf("%d accounts carry uid number %d after the import", holders, n)
	}
}

func TestIntegration_CreateGroup_RefusesATakenGIDNumber(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	first := createTestGroup(t, env, token)
	cn := "zz-gdup-" + uniqueSuffix(t)
	t.Cleanup(func() {
		_, _ = doJSON(t, env, http.MethodDelete,
			groupPath("cn="+cn+","+env.LDAP.GroupBaseDN()), nil, token)
	})

	resp, body := doJSON(t, env, http.MethodPost, "/api/groups/", map[string]any{
		"cn": cn, "gidNumber": first.GIDNumber,
	}, token)
	if resp.StatusCode == http.StatusCreated {
		t.Fatalf("a second group was created on gid number %d", first.GIDNumber)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status=%d, want 409 (body=%s)", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), first.CN) {
		t.Errorf("the message does not name the group already holding it: %s", body)
	}
}

// TestIntegration_Groups_CreateAllocatesGID: a creation that leaves gidNumber
// out gets one allocated at write time, rather than replaying a number the form
// read when the page loaded — which two people creating groups at once would
// both have sent.
func TestIntegration_Groups_CreateAllocatesGID(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	var created []ldap.Group
	t.Cleanup(func() {
		for _, g := range created {
			_, _ = doJSON(t, env, http.MethodDelete, groupPath(g.DN), nil, token)
		}
	})

	// Twice in a row, from the same "page load": both must get a number, and
	// not the same one.
	for i := 0; i < 2; i++ {
		resp, body := doJSON(t, env, http.MethodPost, "/api/groups/",
			map[string]any{"cn": "autogid-" + uniqueSuffix(t)}, token)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create #%d without gidNumber: status=%d body=%s", i+1, resp.StatusCode, body)
		}
		var g ldap.Group
		if err := json.Unmarshal(body, &g); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		created = append(created, g)
		if g.GIDNumber < env.LDAP.MinGID() {
			t.Errorf("allocated gidNumber %d is below the minimum %d", g.GIDNumber, env.LDAP.MinGID())
		}
	}
	if created[0].GIDNumber == created[1].GIDNumber {
		t.Errorf("both groups were allocated gidNumber %d", created[0].GIDNumber)
	}
}

// TestIntegration_CreateUser_AllocatesPrimaryGroupGID: an account created with
// a group of its own and no gidNumber gets one allocated, carried by both the
// account and the group. Before, the form sent a nextGid read at page load,
// and a zero would have been written to the account as is: gidNumber 0 is
// root's group on every host.
func TestIntegration_CreateUser_AllocatesPrimaryGroupGID(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	uid := "zz-pg-" + uniqueSuffix(t)
	t.Cleanup(func() {
		_, _ = doJSON(t, env, http.MethodDelete,
			userPath("uid="+uid+","+env.LDAP.UserBaseDN()), nil, token)
		_, _ = doJSON(t, env, http.MethodDelete,
			groupPath("cn="+uid+","+env.LDAP.GroupBaseDN()), nil, token)
	})

	resp, body := doJSON(t, env, http.MethodPost, "/api/users/", map[string]any{
		"uid": uid, "givenName": "Primary", "sn": "Probe", "createPrimaryGroup": true,
	}, token)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: status=%d body=%s", resp.StatusCode, body)
	}
	var created ldap.User
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if created.GIDNumber < env.LDAP.MinGID() {
		t.Fatalf("account gidNumber = %d, want an allocated one >= %d", created.GIDNumber, env.LDAP.MinGID())
	}
	group, err := env.LDAP.GetGroupByCN(uid)
	if err != nil {
		t.Fatalf("the primary group was not created: %v", err)
	}
	if group.GIDNumber != created.GIDNumber {
		t.Errorf("group gidNumber = %d, account's = %d; they must be the same number",
			group.GIDNumber, created.GIDNumber)
	}
}

// Without a group of its own to create, a zero gidNumber has nothing to be
// allocated for: refused, rather than written as root's group.
func TestIntegration_CreateUser_RefusesZeroGIDWithoutPrimaryGroup(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	uid := "zz-nogid-" + uniqueSuffix(t)
	t.Cleanup(func() {
		_, _ = doJSON(t, env, http.MethodDelete,
			userPath("uid="+uid+","+env.LDAP.UserBaseDN()), nil, token)
	})

	resp, body := doJSON(t, env, http.MethodPost, "/api/users/", map[string]any{
		"uid": uid, "givenName": "No", "sn": "Gid",
	}, token)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400 (body=%s)", resp.StatusCode, body)
	}
	if u, _ := env.LDAP.GetUser("uid=" + uid + "," + env.LDAP.UserBaseDN()); u != nil {
		t.Errorf("the account was created anyway, gidNumber %d", u.GIDNumber)
	}
}

// A primary group asked for on a number another group holds is refused before
// anything is written. The account used to be created first and the group
// second, so the refusal came after the account existed.
func TestIntegration_CreateUser_RefusesATakenPrimaryGroupGID(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	holder := createTestGroup(t, env, token)
	uid := "zz-pgdup-" + uniqueSuffix(t)
	t.Cleanup(func() {
		_, _ = doJSON(t, env, http.MethodDelete,
			userPath("uid="+uid+","+env.LDAP.UserBaseDN()), nil, token)
	})

	resp, body := doJSON(t, env, http.MethodPost, "/api/users/", map[string]any{
		"uid": uid, "givenName": "Dup", "sn": "Primary",
		"gidNumber": holder.GIDNumber, "createPrimaryGroup": true,
	}, token)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status=%d, want 409 (body=%s)", resp.StatusCode, body)
	}
	if u, _ := env.LDAP.GetUser("uid=" + uid + "," + env.LDAP.UserBaseDN()); u != nil {
		t.Errorf("the account was written before the refusal")
	}
}

// usersWithUIDNumber counts the accounts carrying a number, asked of the
// directory rather than of the app: the whole point is that the directory used
// to hold two.
func usersWithUIDNumber(env *testEnv, uidNumber int) (int, error) {
	entries, err := env.LDAP.Search(env.LDAP.UserBaseDN(),
		"(&(objectClass=posixAccount)(uidNumber="+itoa(uidNumber)+"))", []string{"uid"})
	if err != nil {
		return 0, err
	}
	return len(entries), nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
