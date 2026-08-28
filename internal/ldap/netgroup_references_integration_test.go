//go:build integration

package ldap

import (
	"fmt"
	"strings"
	"testing"
)

// seedNetgroup creates a netgroup for a test and removes it afterwards.
func seedNetgroup(t *testing.T, c *Client, cn string, triples ...NetgroupTriple) string {
	t.Helper()
	dn := fmt.Sprintf("cn=%s,%s", cn, c.NetgroupBaseDN())
	_ = c.DeleteNetgroup(dn)
	if _, err := c.CreateNetgroup(CreateNetgroupRequest{CN: cn, Triples: triples}); err != nil {
		t.Fatalf("seed netgroup %q: %v", cn, err)
	}
	t.Cleanup(func() { _ = c.DeleteNetgroup(dn) })
	return dn
}

// seedUser creates a user and cleans up everything it caused, the primary group
// included.
//
// DeleteUser leaves that group behind on purpose — deleting a person is not
// authority to delete a group — so a test that does not remove it makes the next
// run fail on "Entry Already Exists". Both are also cleared up front, so an
// interrupted run does not poison the following one.
func seedUser(t *testing.T, c *Client, uid string) *User {
	t.Helper()
	purgeUser(c, uid)
	next, err := c.NextUID()
	if err != nil {
		t.Fatalf("next uid: %v", err)
	}
	user, err := c.CreateUser(CreateUserRequest{
		UID: uid, GivenName: "Test", SN: "User", CN: uid,
		UIDNumber: next, GIDNumber: next,
		HomeDirectory: "/home/" + uid, LoginShell: "/bin/bash",
		CreatePrimaryGroup: true,
	})
	if err != nil {
		t.Fatalf("create user %q: %v", uid, err)
	}
	t.Cleanup(func() { purgeUser(c, uid) })
	return user
}

// purgeUser removes a user and the primary group created alongside it, ignoring
// whichever of the two is already gone.
func purgeUser(c *Client, uid string) {
	_, _ = c.DeleteUser(fmt.Sprintf("uid=%s,%s", uid, c.UserBaseDN()))
	_ = c.DeleteGroup(fmt.Sprintf("cn=%s,%s", uid, c.GroupBaseDN()))
}

// TestRenameUserMigratesNetgroupTriples is the reason lot 4 exists.
//
// The uid lives inside a "(host,user,domain)" string, so the wholesale value
// swap that migrates memberUid and sudoUser cannot work here. Left unmigrated, a
// renamed account keeps its access under a name nobody holds any more — and the
// next person given that username inherits it.
func TestRenameUserMigratesNetgroupTriples(t *testing.T) {
	client := netgroupTestClient(t)

	user := seedUser(t, client, "ldapwarden-rename-src")
	oldUID := user.UID
	newUID := "ldapwarden-rename-dst"
	t.Cleanup(func() { purgeUser(client, newUID) })

	// Three shapes in one netgroup: the user alone, the user pinned to a host,
	// and two decoys that must not move — a host named after the user, and a
	// longer uid containing it.
	dn := seedNetgroup(t, client, "ldapwarden-rename-ng",
		NetgroupTriple{User: oldUID},
		NetgroupTriple{Host: "web01", User: oldUID},
		NetgroupTriple{Host: oldUID, User: "someone-else"},
		NetgroupTriple{User: oldUID + "-backup"},
	)

	newDN, warnings, err := client.RenameUser(user.DN, oldUID, newUID)
	if err != nil {
		t.Fatalf("RenameUser: %v", err)
	}
	if len(warnings) != 0 {
		t.Logf("rename warnings: %v", warnings)
	}
	if newDN == "" {
		t.Fatal("RenameUser returned an empty DN")
	}

	after, err := client.GetNetgroup(dn)
	if err != nil {
		t.Fatalf("GetNetgroup: %v", err)
	}

	want := []NetgroupTriple{
		{User: newUID},
		{Host: "web01", User: newUID},
		{Host: oldUID, User: "someone-else"}, // the host keeps the old name
		{User: oldUID + "-backup"},           // a different account entirely
	}
	if len(after.Triples) != len(want) {
		t.Fatalf("triples = %v, want %d", after.Triples, len(want))
	}
	got := map[NetgroupTriple]bool{}
	for _, triple := range after.Triples {
		got[triple] = true
	}
	for _, triple := range want {
		if !got[triple] {
			t.Errorf("missing %v after rename; got %v", triple, after.Triples)
		}
	}

	// And the lookups agree: nothing still reaches the old name.
	if stale, err := client.NetgroupsForUser(oldUID); err != nil {
		t.Fatalf("NetgroupsForUser: %v", err)
	} else if len(stale) != 0 {
		t.Errorf("the old uid still resolves to %d netgroup(s)", len(stale))
	}
	if fresh, err := client.NetgroupsForUser(newUID); err != nil {
		t.Fatalf("NetgroupsForUser: %v", err)
	} else if len(fresh) != 1 {
		t.Errorf("the new uid resolves to %d netgroup(s), want 1", len(fresh))
	}
}

// TestDeleteUserRemovesNetgroupTriples covers the other half, including the
// warning for a netgroup left with nothing in it.
func TestDeleteUserRemovesNetgroupTriples(t *testing.T) {
	client := netgroupTestClient(t)

	user := seedUser(t, client, "ldapwarden-delete-src")
	uid := user.UID

	shared := seedNetgroup(t, client, "ldapwarden-delete-shared",
		NetgroupTriple{User: uid},
		NetgroupTriple{User: "someone-else"},
		NetgroupTriple{Host: uid, User: "someone-else"},
	)
	solo := seedNetgroup(t, client, "ldapwarden-delete-solo",
		NetgroupTriple{User: uid},
	)

	warnings, err := client.DeleteUser(user.DN)
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	afterShared, err := client.GetNetgroup(shared)
	if err != nil {
		t.Fatalf("GetNetgroup(shared): %v", err)
	}
	if len(afterShared.Triples) != 2 {
		t.Errorf("shared netgroup = %v, want the two triples not naming %q",
			afterShared.Triples, uid)
	}
	for _, triple := range afterShared.Triples {
		if triple.NamesUser(uid) {
			t.Errorf("triple %v still names the deleted user", triple)
		}
	}

	afterSolo, err := client.GetNetgroup(solo)
	if err != nil {
		t.Fatalf("GetNetgroup(solo): %v", err)
	}
	if len(afterSolo.Triples) != 0 {
		t.Errorf("solo netgroup = %v, want empty", afterSolo.Triples)
	}
	// Emptied, not deleted — and said out loud, like a sudo role that named only
	// this user.
	var warned bool
	for _, w := range warnings {
		if strings.Contains(w, "ldapwarden-delete-solo") && strings.Contains(w, "now empty") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("no warning about the emptied netgroup; warnings = %v", warnings)
	}
}
