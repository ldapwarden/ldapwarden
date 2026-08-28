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
