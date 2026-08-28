//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// TestIntegration_GroupMembers_ReportLockout covers the membership list saying
// which of its members cannot use what the group grants.
//
// The endpoint used to return uid, cn, displayName and hasPhoto, so an account
// an administrator had locked months earlier sat in the list looking exactly
// like an account in daily use — on the one screen where that matters, since a
// membership review is where dead access is supposed to be spotted.
//
// Asserted through the HTTP layer rather than against GroupMembers directly:
// the browser reads these names off the JSON and validates them with a Zod
// schema, so a field that stops being serialised is a silently broken UI.
func TestIntegration_GroupMembers_ReportLockout(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	locked := createTestUser(t, env, token)
	active := createTestUser(t, env, token)
	group := createTestGroup(t, env, token)

	for _, uid := range []string{locked.UID, active.UID} {
		resp, body := doJSON(t, env, http.MethodPost, groupPath(group.DN)+"/members",
			map[string]string{"memberUid": uid}, token)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("add member %q: status=%d body=%s", uid, resp.StatusCode, body)
		}
	}

	// Lock one of them the way an administrator does, through the API, so the
	// entry carries whatever value the app itself writes rather than one this
	// test invented.
	resp, body := doJSON(t, env, http.MethodPost, userPath(locked.DN)+"/lock", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("lock user: status=%d body=%s", resp.StatusCode, body)
	}

	_, body = doJSON(t, env, http.MethodGet, groupPath(group.DN)+"/members", nil, token)
	var page struct {
		Data []ldap.GroupMember `json:"data"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("unmarshal members: %v (body=%s)", err, body)
	}

	byUID := make(map[string]ldap.GroupMember, len(page.Data))
	for _, member := range page.Data {
		byUID[member.UID] = member
	}

	lockedMember, ok := byUID[locked.UID]
	if !ok {
		t.Fatalf("locked member %q missing from the list", locked.UID)
	}
	if lockedMember.Lockout == nil {
		t.Fatal("locked member carries no lockout: the list cannot flag it")
	}
	// Specifically the administrative state, not merely "locked". A ppolicy
	// lockout clears itself and means something else entirely to whoever is
	// reviewing the membership.
	if got := lockedMember.Lockout.State; got != ldap.LockStateManual {
		t.Errorf("locked member state = %q, want %q", got, ldap.LockStateManual)
	}
	if !lockedMember.Lockout.Locked() {
		t.Error("locked member reports Locked() false")
	}

	activeMember, ok := byUID[active.UID]
	if !ok {
		t.Fatalf("active member %q missing from the list", active.UID)
	}
	if activeMember.Lockout != nil && activeMember.Lockout.Locked() {
		t.Errorf("untouched member reports locked (state %q)", activeMember.Lockout.State)
	}

	// The field has to survive JSON, not just exist on the struct: the UI reads
	// it by name.
	var raw struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	for _, member := range raw.Data {
		if member["uid"] != locked.UID {
			continue
		}
		lockout, ok := member["lockout"].(map[string]any)
		if !ok {
			t.Fatalf("lockout absent from the JSON for %q: %v", locked.UID, member)
		}
		if lockout["state"] != string(ldap.LockStateManual) {
			t.Errorf("JSON lockout.state = %v, want %q", lockout["state"], ldap.LockStateManual)
		}
	}
}
