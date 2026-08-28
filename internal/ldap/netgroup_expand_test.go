package ldap

import (
	"reflect"
	"sort"
	"testing"
	"time"
)

// resolverFor builds a resolver over a graph held in memory. The cases worth
// testing here — a cycle, a reference into nothing — are ones a validating
// directory will not hold on request, so they are constructed rather than
// seeded.
func resolverFor(netgroups ...Netgroup) *netgroupResolver {
	r := &netgroupResolver{}
	r.index(netgroups)
	return r
}

func ng(cn string, triples []NetgroupTriple, members ...string) Netgroup {
	return Netgroup{CN: cn, Triples: triples, Members: members}
}

func user(uid string) NetgroupTriple { return NetgroupTriple{User: uid} }
func host(h string) NetgroupTriple   { return NetgroupTriple{Host: h} }

func memberNames(members []NetgroupMember) []string {
	names := make([]string, 0, len(members))
	for _, member := range members {
		names = append(names, member.Name)
	}
	return names
}

func TestExpandFollowsNesting(t *testing.T) {
	r := resolverFor(
		ng("ops-oncall", []NetgroupTriple{user("asmith")}, "ops"),
		ng("ops", []NetgroupTriple{user("admin"), user("jdoe")}),
	)

	got, err := r.expand("ops-oncall")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}

	want := []string{"admin", "asmith", "jdoe"}
	if names := memberNames(got.Users); !reflect.DeepEqual(names, want) {
		t.Errorf("Users = %v, want %v", names, want)
	}

	// Attribution: asmith is named by the root, the other two arrive through ops.
	byName := map[string]string{}
	for _, member := range got.Users {
		byName[member.Name] = member.Via
	}
	if byName["asmith"] != "" {
		t.Errorf("asmith Via = %q, want empty (named directly)", byName["asmith"])
	}
	if byName["jdoe"] != "ops" {
		t.Errorf("jdoe Via = %q, want %q", byName["jdoe"], "ops")
	}
}

// TestExpandOpenSlotIsNotEmpty is the trap this type exists for: a netgroup with
// no host triples and one whose host slots are all open both produce an empty
// Hosts, and they mean opposite things.
func TestExpandOpenSlotIsNotEmpty(t *testing.T) {
	// "(,jdoe,)" — a user netgroup. The host slot is open, so it matches every
	// host, and reporting "no hosts" would be the exact inverse of the truth.
	open := resolverFor(ng("ops", []NetgroupTriple{user("jdoe")}))
	got, err := open.expand("ops")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if len(got.Hosts) != 0 {
		t.Errorf("Hosts = %v, want none named", got.Hosts)
	}
	if !got.AnyHost() {
		t.Error("AnyHost() is false, but the host slot is open: the netgroup matches every host")
	}
	if !reflect.DeepEqual(got.OpenHost, []string{"ops"}) {
		t.Errorf("OpenHost = %v, want [ops]", got.OpenHost)
	}
	if got.AnyUser() {
		t.Error("AnyUser() is true, but every user slot names someone")
	}

	// A netgroup that genuinely contributes no hosts: the slot is "-", which
	// matches nothing.
	none := resolverFor(ng("nobody", []NetgroupTriple{{Host: NetgroupWildcard, User: "jdoe"}}))
	got, err = none.expand("nobody")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if len(got.Hosts) != 0 || got.AnyHost() {
		t.Errorf("a %q slot contributed something: Hosts=%v AnyHost=%v",
			NetgroupWildcard, got.Hosts, got.AnyHost())
	}
}

// TestExpandOpenSlotInheritedThroughNesting: the open slot may be several levels
// down, and the flag has to survive the walk with the name of whoever opened it.
func TestExpandOpenSlotInheritedThroughNesting(t *testing.T) {
	r := resolverFor(
		ng("top", []NetgroupTriple{host("web01")}, "middle"),
		ng("middle", nil, "bottom"),
		ng("bottom", []NetgroupTriple{user("jdoe")}),
	)

	got, err := r.expand("top")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if !got.AnyHost() {
		t.Error("AnyHost() is false: bottom's open host slot was lost through nesting")
	}
	if !reflect.DeepEqual(got.OpenHost, []string{"bottom"}) {
		t.Errorf("OpenHost = %v, want [bottom] — the netgroup that opened it", got.OpenHost)
	}
	// top's own triple opens the user slot, since "(web01,,)" names no user.
	if !reflect.DeepEqual(got.OpenUser, []string{"top"}) {
		t.Errorf("OpenUser = %v, want [top]", got.OpenUser)
	}
}

func TestExpandCycles(t *testing.T) {
	cases := []struct {
		name  string
		graph []Netgroup
		root  string
		users []string
	}{
		{
			name: "direct cycle",
			graph: []Netgroup{
				ng("a", []NetgroupTriple{user("alice")}, "b"),
				ng("b", []NetgroupTriple{user("bob")}, "a"),
			},
			root:  "a",
			users: []string{"alice", "bob"},
		},
		{
			name: "self reference",
			graph: []Netgroup{
				ng("a", []NetgroupTriple{user("alice")}, "a"),
			},
			root:  "a",
			users: []string{"alice"},
		},
		{
			name: "indirect cycle",
			graph: []Netgroup{
				ng("a", []NetgroupTriple{user("alice")}, "b"),
				ng("b", []NetgroupTriple{user("bob")}, "c"),
				ng("c", []NetgroupTriple{user("carol")}, "a"),
			},
			root:  "a",
			users: []string{"alice", "bob", "carol"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := resolverFor(tc.graph...)

			// A cycle must not hang. If the visited set ever stops working this
			// deadline is what says so, rather than the suite timing out.
			done := make(chan *NetgroupExpansion, 1)
			go func() {
				got, err := r.expand(tc.root)
				if err != nil {
					t.Errorf("expand: %v", err)
					done <- nil
					return
				}
				done <- got
			}()

			select {
			case got := <-done:
				if got == nil {
					return
				}
				if names := memberNames(got.Users); !reflect.DeepEqual(names, tc.users) {
					t.Errorf("Users = %v, want %v", names, tc.users)
				}
				// The loop is reported, not treated as a failure: innetgr() bounds
				// it too, so the hosts apply this configuration without complaint.
				if len(got.Cycles) == 0 {
					t.Error("the cycle was not reported")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("expand did not terminate on a cycle")
			}
		})
	}
}

// TestExpandDiamond: reached by two paths, a member is counted once and
// attributed to the nearer one — which is why the walk is breadth-first.
func TestExpandDiamond(t *testing.T) {
	r := resolverFor(
		ng("top", nil, "left", "right"),
		ng("left", nil, "shared"),
		ng("right", []NetgroupTriple{user("shared-user")}, "shared"),
		ng("shared", []NetgroupTriple{user("shared-user")}),
	)

	got, err := r.expand("top")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}

	if names := memberNames(got.Users); !reflect.DeepEqual(names, []string{"shared-user"}) {
		t.Errorf("Users = %v, want it counted once", names)
	}
	// right is one hop from the root, shared is two, so the nearer attribution
	// wins.
	if got.Users[0].Via != "right" {
		t.Errorf("Via = %q, want %q — the nearest contributor", got.Users[0].Via, "right")
	}
}

func TestExpandDangling(t *testing.T) {
	r := resolverFor(
		ng("ops", []NetgroupTriple{user("jdoe")}, "gone", "also-gone"),
	)

	got, err := r.expand("ops")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	// Surfaced, not skipped: a reference into nothing is the operator's to fix.
	if !reflect.DeepEqual(got.Dangling, []string{"also-gone", "gone"}) {
		t.Errorf("Dangling = %v, want both missing names", got.Dangling)
	}
	if names := memberNames(got.Users); !reflect.DeepEqual(names, []string{"jdoe"}) {
		t.Errorf("Users = %v: the dangling references disturbed the walk", names)
	}
}

func TestExpandDepth(t *testing.T) {
	// Deep enough that any hidden recursion limit would show, and every level
	// contributes so a truncated walk cannot pass unnoticed.
	const depth = 200
	graph := make([]Netgroup, 0, depth)
	want := make([]string, 0, depth)
	for i := 0; i < depth; i++ {
		name := "ng" + itoa(i)
		uid := "user" + itoa(i)
		want = append(want, uid)
		if i+1 < depth {
			graph = append(graph, ng(name, []NetgroupTriple{user(uid)}, "ng"+itoa(i+1)))
		} else {
			graph = append(graph, ng(name, []NetgroupTriple{user(uid)}))
		}
	}

	got, err := resolverFor(graph...).expand("ng0")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if len(got.Users) != depth {
		t.Fatalf("reached %d users at depth %d: the walk was truncated", len(got.Users), depth)
	}
	if len(got.Cycles) != 0 {
		t.Errorf("a chain was reported as cyclic: %v", got.Cycles)
	}
	sort.Strings(want)
	if names := memberNames(got.Users); !reflect.DeepEqual(names, want) {
		t.Errorf("the walk reached the wrong users at depth %d", depth)
	}
}

func TestExpandUnknownRoot(t *testing.T) {
	if _, err := resolverFor().expand("nope"); err == nil {
		t.Error("expanding an unknown netgroup succeeded, want an error")
	}
}

// TestContainingUserWalksUpwards covers the reverse direction, which is what
// sudo will need: "+ops-oncall" grants to everyone ops-oncall absorbs, so a user
// named only by a nested netgroup is still reached.
func TestContainingUserWalksUpwards(t *testing.T) {
	r := resolverFor(
		ng("ops-oncall", []NetgroupTriple{user("asmith")}, "ops"),
		ng("ops", []NetgroupTriple{user("jdoe")}),
		ng("unrelated", []NetgroupTriple{user("carol")}),
		// A netgroup naming jdoe in the *host* slot must not be picked up.
		ng("hosts", []NetgroupTriple{host("jdoe")}),
	)

	got, err := r.containingUser("jdoe")
	if err != nil {
		t.Fatalf("containingUser: %v", err)
	}
	if want := []string{"ops", "ops-oncall"}; !reflect.DeepEqual(got, want) {
		t.Errorf("containingUser(jdoe) = %v, want %v", got, want)
	}

	if got, err = r.containingUser("carol"); err != nil {
		t.Fatalf("containingUser: %v", err)
	}
	if want := []string{"unrelated"}; !reflect.DeepEqual(got, want) {
		t.Errorf("containingUser(carol) = %v, want %v", got, want)
	}

	if got, err = r.containingUser("nobody"); err != nil {
		t.Fatalf("containingUser: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("containingUser(nobody) = %v, want none", got)
	}
}

// TestContainingUserTerminatesOnCycle: the upward walk has the same loop
// exposure as the downward one.
func TestContainingUserTerminatesOnCycle(t *testing.T) {
	r := resolverFor(
		ng("a", []NetgroupTriple{user("alice")}, "b"),
		ng("b", nil, "a"),
	)

	done := make(chan []string, 1)
	go func() {
		got, err := r.containingUser("alice")
		if err != nil {
			t.Errorf("containingUser: %v", err)
		}
		done <- got
	}()

	select {
	case got := <-done:
		if want := []string{"a", "b"}; !reflect.DeepEqual(got, want) {
			t.Errorf("containingUser(alice) = %v, want %v", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("containingUser did not terminate on a cycle")
	}
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
