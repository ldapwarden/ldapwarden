//go:build integration

package ldap

import "testing"

// TestExpandSeededNesting runs the expansion against the directory rather than a
// graph built in memory, so the seeded ops-oncall → ops edge is walked for real.
func TestExpandSeededNesting(t *testing.T) {
	client := netgroupTestClient(t)

	got, err := client.ExpandNetgroup("ops-oncall")
	if err != nil {
		t.Fatalf("ExpandNetgroup: %v", err)
	}

	want := map[string]string{
		"asmith": "",    // named by ops-oncall itself
		"admin":  "ops", // absorbed from ops
		"jdoe":   "ops",
	}
	if len(got.Users) != len(want) {
		t.Fatalf("Users = %v, want %d entries", got.Users, len(want))
	}
	for _, member := range got.Users {
		via, ok := want[member.Name]
		if !ok {
			t.Errorf("unexpected user %q", member.Name)
			continue
		}
		if member.Via != via {
			t.Errorf("%s Via = %q, want %q", member.Name, member.Via, via)
		}
	}

	// Both seeded netgroups leave the host slot open, so this matches every host
	// — the case a bare list of hosts would report as "none".
	if !got.AnyHost() {
		t.Error("AnyHost() is false, but every seeded triple leaves the host slot open")
	}
	if len(got.Hosts) != 0 {
		t.Errorf("Hosts = %v, want no host named", got.Hosts)
	}
	if len(got.Cycles) != 0 || len(got.Dangling) != 0 {
		t.Errorf("clean seed reported problems: cycles=%v dangling=%v", got.Cycles, got.Dangling)
	}

	// A host netgroup is the mirror image.
	hosts, err := client.ExpandNetgroup("web-servers")
	if err != nil {
		t.Fatalf("ExpandNetgroup(web-servers): %v", err)
	}
	if names := len(hosts.Hosts); names != 2 {
		t.Errorf("web-servers has %d hosts, want 2", names)
	}
	if !hosts.AnyUser() {
		t.Error("AnyUser() is false, but web-servers leaves the user slot open")
	}
}

// TestNetgroupsContainingUserThroughNesting is the lookup sudo will need: a user
// named only by a nested netgroup is still granted by the parent.
func TestNetgroupsContainingUserThroughNesting(t *testing.T) {
	client := netgroupTestClient(t)

	got, err := client.NetgroupsContainingUser("jdoe")
	if err != nil {
		t.Fatalf("NetgroupsContainingUser: %v", err)
	}
	// Direct in ops, and reached through nesting from ops-oncall. Compare with
	// NetgroupsForUser, which returns ops alone.
	want := map[string]bool{"ops": true, "ops-oncall": true}
	if len(got) != len(want) {
		t.Fatalf("NetgroupsContainingUser(jdoe) = %v, want %v", got, want)
	}
	for _, name := range got {
		if !want[name] {
			t.Errorf("unexpected netgroup %q", name)
		}
	}

	direct, err := client.NetgroupsForUser("jdoe")
	if err != nil {
		t.Fatalf("NetgroupsForUser: %v", err)
	}
	if len(direct) != 1 || direct[0].CN != "ops" {
		t.Errorf("NetgroupsForUser(jdoe) = %v, want [ops] — direct membership only", direct)
	}
}
