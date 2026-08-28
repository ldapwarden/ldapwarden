//go:build integration

package ldap

import (
	"os"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/config"
)

func netgroupTestClient(t *testing.T) *Client {
	t.Helper()
	if os.Getenv("LDAPWARDEN_TEST_INTEGRATION") != "1" {
		t.Skip("set LDAPWARDEN_TEST_INTEGRATION=1 (and start docker compose) to run integration tests")
	}
	client := NewClient(config.LDAPConfig{
		URL:        "ldap://localhost:389",
		BaseDN:     "dc=example,dc=org",
		BindDN:     "cn=admin,dc=example,dc=org",
		BindPass:   "admin",
		UserOU:     "ou=People",
		GroupOU:    "ou=Groups",
		SudoersOU:  "ou=sudoers",
		PpolicyOU:  "ou=policies",
		NetgroupOU: "ou=netgroups",
	})
	t.Cleanup(client.Close)
	if _, err := client.GetEntry("dc=example,dc=org", []string{"dc"}); err != nil {
		t.Skipf("directory unreachable: %v", err)
	}
	return client
}

// TestNetgroupCapability pins the gate the whole module hangs off. The bundled
// image loads nis.schema, so this must answer true here; on a directory without
// it the UI hides netgroups rather than failing at save time.
func TestNetgroupCapability(t *testing.T) {
	client := netgroupTestClient(t)

	if !client.Capabilities().Netgroups {
		t.Error("Capabilities().Netgroups is false, but the bundled directory loads nis.schema")
	}
	// The probe must not be fooled by a name that is not declared.
	if client.hasObjectClass("nisNetgroupThatDoesNotExist") {
		t.Error("hasObjectClass reported an undeclared class")
	}
	// And the attribute probe must still work after being generalised.
	if !client.hasAttributeType(pwdEndTimeAttr) {
		t.Error("hasAttributeType regressed: pwdEndTime not found")
	}
}

// TestNetgroupReadSeed reads what the seed puts in the directory, which is the
// first real exercise of the parser against values it did not produce itself.
func TestNetgroupReadSeed(t *testing.T) {
	client := netgroupTestClient(t)

	ops, err := client.GetNetgroupByCN("ops")
	if err != nil {
		t.Fatalf("GetNetgroupByCN(ops): %v", err)
	}
	if len(ops.Malformed) != 0 {
		t.Errorf("seeded netgroup has unparsed triples: %v", ops.Malformed)
	}
	if got := len(ops.Triples); got != 2 {
		t.Fatalf("ops has %d triples, want 2", got)
	}
	// A user netgroup: the host slot is open, which is what makes it usable from
	// sudoUser and useless from sudoHost.
	for _, triple := range ops.Triples {
		if triple.Host != "" {
			t.Errorf("ops triple %v has a host, expected a user netgroup", triple)
		}
		if triple.User == "" {
			t.Errorf("ops triple %v has no user", triple)
		}
	}
	if got := ops.Kind(); got != NetgroupKindUsers {
		t.Errorf("ops Kind() = %q, want %q", got, NetgroupKindUsers)
	}

	hosts, err := client.GetNetgroupByCN("web-servers")
	if err != nil {
		t.Fatalf("GetNetgroupByCN(web-servers): %v", err)
	}
	if got := hosts.Kind(); got != NetgroupKindHosts {
		t.Errorf("web-servers Kind() = %q, want %q", got, NetgroupKindHosts)
	}

	nested, err := client.GetNetgroupByCN("ops-oncall")
	if err != nil {
		t.Fatalf("GetNetgroupByCN(ops-oncall): %v", err)
	}
	if len(nested.Members) != 1 || nested.Members[0] != "ops" {
		t.Errorf("ops-oncall Members = %v, want [ops]", nested.Members)
	}
}

// TestNetgroupsForUser is the lookup the rename path will depend on, and the one
// place a substring filter could quietly get it wrong.
func TestNetgroupsForUser(t *testing.T) {
	client := netgroupTestClient(t)

	found, err := client.NetgroupsForUser("jdoe")
	if err != nil {
		t.Fatalf("NetgroupsForUser: %v", err)
	}
	var names []string
	for _, netgroup := range found {
		names = append(names, netgroup.CN)
	}
	// Direct membership only: ops-oncall reaches jdoe through nesting and must
	// not appear here.
	if len(names) != 1 || names[0] != "ops" {
		t.Errorf("NetgroupsForUser(jdoe) = %v, want [ops]", names)
	}

	// A host of the same name must not be mistaken for the user. web01 sits in
	// the host slot of web-servers, so asking for the *user* web01 finds nothing.
	if found, err = client.NetgroupsForUser("web01"); err != nil {
		t.Fatalf("NetgroupsForUser(web01): %v", err)
	}
	if len(found) != 0 {
		t.Errorf("NetgroupsForUser(web01) matched the host slot: %v", found)
	}

	if found, err = client.NetgroupsForHost("web01"); err != nil {
		t.Fatalf("NetgroupsForHost(web01): %v", err)
	}
	if len(found) != 1 || found[0].CN != "web-servers" {
		t.Errorf("NetgroupsForHost(web01) = %v, want [web-servers]", found)
	}
}

// TestNetgroupCRUD exercises the write path, including the two things most
// likely to be got wrong: a triple list replaced wholesale, and a malformed
// value that has to survive a save it was not part of.
func TestNetgroupCRUD(t *testing.T) {
	client := netgroupTestClient(t)

	const cn = "ldapwarden-test-netgroup"
	dn := "cn=" + cn + ",ou=netgroups,dc=example,dc=org"
	_ = client.DeleteNetgroup(dn) // from an interrupted earlier run
	t.Cleanup(func() { _ = client.DeleteNetgroup(dn) })

	created, err := client.CreateNetgroup(CreateNetgroupRequest{
		CN:          cn,
		Description: "created by the test suite",
		Triples: []NetgroupTriple{
			{User: "jdoe"},
			{Host: "web01"},
		},
		Members: []string{"ops"},
	})
	if err != nil {
		t.Fatalf("CreateNetgroup: %v", err)
	}
	if len(created.Triples) != 2 {
		t.Errorf("created with %d triples, want 2", len(created.Triples))
	}
	if got := created.Kind(); got != NetgroupKindMixed {
		t.Errorf("Kind() = %q, want %q for a netgroup with both slots used", got, NetgroupKindMixed)
	}

	updated, err := client.UpdateNetgroup(dn, UpdateNetgroupRequest{
		Triples: []NetgroupTriple{{User: "alice"}},
	})
	if err != nil {
		t.Fatalf("UpdateNetgroup: %v", err)
	}
	if len(updated.Triples) != 1 || !updated.Triples[0].NamesUser("alice") {
		t.Errorf("after update, triples = %v, want one naming alice", updated.Triples)
	}
	// Members were not part of the request, so they must be untouched.
	if len(updated.Members) != 1 || updated.Members[0] != "ops" {
		t.Errorf("update clobbered members: %v", updated.Members)
	}

	if err := client.DeleteNetgroup(dn); err != nil {
		t.Fatalf("DeleteNetgroup: %v", err)
	}
	if _, err := client.GetNetgroup(dn); err == nil {
		t.Error("netgroup still readable after delete")
	}
}
