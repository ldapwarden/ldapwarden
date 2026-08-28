package ldap

import (
	"fmt"
	"strings"
)

// NetgroupTriple is one nisNetgroupTriple value, parsed.
//
// The stored form is "(host,user,domain)". Each slot is independent and any of
// them may be empty, which in NIS means "any" — so "(,jdoe,)" is the user jdoe
// on every host, and "(web01,,)" is every user on web01. That asymmetry is the
// whole reason a netgroup can be used from sudoUser or from sudoHost and mean
// entirely different things.
type NetgroupTriple struct {
	Host   string `json:"host"`
	User   string `json:"user"`
	Domain string `json:"domain"`
}

// NetgroupWildcard is the value NIS gives a slot that must match nothing at all,
// as opposed to an empty slot, which matches everything.
//
// The two are not interchangeable and the difference is easy to lose: a slot
// silently turned from "-" into "" widens a grant instead of narrowing it. Every
// rewrite in this package therefore leaves it alone.
const NetgroupWildcard = "-"

// ParseNetgroupTriple reads one stored nisNetgroupTriple value.
//
// Surrounding whitespace, and whitespace around each slot, is tolerated for
// portability. OpenLDAP will not produce it — it validates this attribute
// against SYNTAX 1.3.6.1.1.1.0.0 and refuses "( a , b , c )" — but a directory
// that does not validate can hold one, and refusing to display an entry the
// server accepted helps nobody.
//
// Anything else — missing parentheses, the wrong number of commas — is reported
// as an error rather than guessed at, because a triple this cannot read is one
// it must not rewrite either. See Netgroup.Malformed.
func ParseNetgroupTriple(raw string) (NetgroupTriple, error) {
	value := strings.TrimSpace(raw)

	if !strings.HasPrefix(value, "(") || !strings.HasSuffix(value, ")") {
		return NetgroupTriple{}, fmt.Errorf("netgroup triple %q is not wrapped in parentheses", raw)
	}

	inner := value[1 : len(value)-1]
	parts := strings.Split(inner, ",")
	if len(parts) != 3 {
		return NetgroupTriple{}, fmt.Errorf(
			"netgroup triple %q has %d comma-separated fields, want 3 (host,user,domain)", raw, len(parts))
	}

	return NetgroupTriple{
		Host:   strings.TrimSpace(parts[0]),
		User:   strings.TrimSpace(parts[1]),
		Domain: strings.TrimSpace(parts[2]),
	}, nil
}

// String renders the triple in the canonical form, without spaces.
//
// Round-tripping a hand-written value therefore normalises it. That is
// deliberate — one spelling in the directory is worth more than preserving
// someone's spacing — but it does mean a save rewrites every triple on the
// entry, so callers that only meant to change one should send the rest back
// unchanged rather than re-deriving them.
func (t NetgroupTriple) String() string {
	return "(" + t.Host + "," + t.User + "," + t.Domain + ")"
}

// IsEmpty reports a triple with nothing in any slot: "(,,)" grants everything to
// everyone, which is almost never meant and is worth refusing at the edge.
func (t NetgroupTriple) IsEmpty() bool {
	return t.Host == "" && t.User == "" && t.Domain == ""
}

// NamesUser reports whether this triple names exactly this user, as opposed to
// leaving the slot open (any user) or excluding it ("-").
//
// This is the test the rename and delete paths need: an empty slot must not be
// filled in with the uid being renamed, and a "-" must not be turned into one.
func (t NetgroupTriple) NamesUser(uid string) bool {
	return uid != "" && t.User == uid
}

// NamesHost is the host-slot counterpart of NamesUser.
func (t NetgroupTriple) NamesHost(host string) bool {
	return host != "" && t.Host == host
}

// WithUser returns a copy with the user slot replaced. Used by RenameUser, which
// cannot swap the stored value wholesale: the uid is a field inside the triple,
// so a naive string replacement would also rewrite a host that happens to
// contain the old uid.
func (t NetgroupTriple) WithUser(uid string) NetgroupTriple {
	t.User = uid
	return t
}
