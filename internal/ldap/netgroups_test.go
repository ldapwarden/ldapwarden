package ldap

import (
	"testing"

	"github.com/go-ldap/ldap/v3"
)

// TestEntryToNetgroupKeepsUnparsableValues covers Netgroup.Malformed.
//
// Deliberately a unit test: OpenLDAP validates the nisNetgroupTriple syntax and
// refuses anything this parser would reject, so the situation cannot be created
// against the bundled directory. It can still arrive from a server that does not
// validate, or from a NIS import, and the rule then matters — a value the app
// cannot read must not be silently dropped by a save that was about something
// else, and the rename path has to report it as a leftover rather than pretend
// it migrated it.
func TestEntryToNetgroupKeepsUnparsableValues(t *testing.T) {
	entry := ldap.NewEntry("cn=odd,ou=netgroups,dc=example,dc=org", map[string][]string{
		"cn":               {"odd"},
		"description":      {"planted by another tool"},
		netgroupTripleAttr: {"(,jdoe,)", "not-a-triple", "(web01,,)", "(a,b)"},
		netgroupMemberAttr: {"ops"},
	})

	netgroup := entryToNetgroup(entry)

	if got := len(netgroup.Triples); got != 2 {
		t.Fatalf("parsed %d triples, want 2 (the two well-formed ones)", got)
	}
	if !netgroup.Triples[0].NamesUser("jdoe") || !netgroup.Triples[1].NamesHost("web01") {
		t.Errorf("parsed the wrong triples: %v", netgroup.Triples)
	}

	want := []string{"not-a-triple", "(a,b)"}
	if len(netgroup.Malformed) != len(want) {
		t.Fatalf("Malformed = %v, want %v", netgroup.Malformed, want)
	}
	for i, value := range want {
		if netgroup.Malformed[i] != value {
			t.Errorf("Malformed[%d] = %q, want %q", i, netgroup.Malformed[i], value)
		}
	}

	// A netgroup whose only readable triples are one user and one host is mixed,
	// and the unreadable values must not have swayed that.
	if got := netgroup.Kind(); got != NetgroupKindMixed {
		t.Errorf("Kind() = %q, want %q", got, NetgroupKindMixed)
	}
}

func TestRenameUserInTriples(t *testing.T) {
	cases := []struct {
		name        string
		in          []NetgroupTriple
		want        []NetgroupTriple
		wantChanged bool
	}{
		{
			name:        "the user slot is rewritten",
			in:          []NetgroupTriple{{User: "jdoe"}},
			want:        []NetgroupTriple{{User: "jsmith"}},
			wantChanged: true,
		},
		{
			name:        "the host slot is left alone",
			in:          []NetgroupTriple{{Host: "web01", User: "jdoe"}},
			want:        []NetgroupTriple{{Host: "web01", User: "jsmith"}},
			wantChanged: true,
		},
		{
			// The bug a wholesale value swap would introduce: a host that happens
			// to be named after the user.
			name:        "a host named like the user is not touched",
			in:          []NetgroupTriple{{Host: "jdoe", User: "alice"}},
			want:        []NetgroupTriple{{Host: "jdoe", User: "alice"}},
			wantChanged: false,
		},
		{
			// The bug a substring replacement would introduce.
			name:        "a longer uid containing the old one is not touched",
			in:          []NetgroupTriple{{User: "jdoe-backup"}},
			want:        []NetgroupTriple{{User: "jdoe-backup"}},
			wantChanged: false,
		},
		{
			// An open slot means "any user". Filling it in with the new uid would
			// narrow the grant from everyone to one person.
			name:        "an open slot is not filled in",
			in:          []NetgroupTriple{{Host: "web01"}},
			want:        []NetgroupTriple{{Host: "web01"}},
			wantChanged: false,
		},
		{
			// And "-" means "nobody". Turning it into a name would invert it.
			name:        "the no-match marker is not turned into a name",
			in:          []NetgroupTriple{{User: NetgroupWildcard}},
			want:        []NetgroupTriple{{User: NetgroupWildcard}},
			wantChanged: false,
		},
		{
			name: "a mixed list rewrites only what names the user",
			in: []NetgroupTriple{
				{User: "jdoe"}, {User: "alice"}, {Host: "jdoe"}, {Host: "web01", User: "jdoe"},
			},
			want: []NetgroupTriple{
				{User: "jsmith"}, {User: "alice"}, {Host: "jdoe"}, {Host: "web01", User: "jsmith"},
			},
			wantChanged: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := renameUserInTriples(tc.in, "jdoe", "jsmith")
			if changed != tc.wantChanged {
				t.Errorf("changed = %v, want %v", changed, tc.wantChanged)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d triples, want %d", len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("triple %d = %v, want %v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestRemoveUserFromTriples(t *testing.T) {
	cases := []struct {
		name        string
		in          []NetgroupTriple
		want        []NetgroupTriple
		wantChanged bool
	}{
		{
			name:        "a user triple is dropped",
			in:          []NetgroupTriple{{User: "jdoe"}, {User: "alice"}},
			want:        []NetgroupTriple{{User: "alice"}},
			wantChanged: true,
		},
		{
			// The whole statement goes: "jdoe on web01" does not separately assert
			// that web01 belongs here, and blanking the user slot would widen it to
			// "anyone on web01".
			name:        "a host+user triple goes entirely",
			in:          []NetgroupTriple{{Host: "web01", User: "jdoe"}},
			want:        []NetgroupTriple{},
			wantChanged: true,
		},
		{
			name:        "an open slot survives: it is not this user",
			in:          []NetgroupTriple{{Host: "web01"}},
			want:        []NetgroupTriple{{Host: "web01"}},
			wantChanged: false,
		},
		{
			name:        "a host named like the user survives",
			in:          []NetgroupTriple{{Host: "jdoe", User: "alice"}},
			want:        []NetgroupTriple{{Host: "jdoe", User: "alice"}},
			wantChanged: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := removeUserFromTriples(tc.in, "jdoe")
			if changed != tc.wantChanged {
				t.Errorf("changed = %v, want %v", changed, tc.wantChanged)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("triple %d = %v, want %v", i, got[i], tc.want[i])
				}
			}
		})
	}
}
