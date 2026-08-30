package ldap

import (
	"fmt"
	"sort"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

// netgroupObjectClass is the structural class from nis.schema. Its presence is
// what Capabilities.Netgroups reports.
const netgroupObjectClass = "nisNetgroup"

const (
	netgroupTripleAttr = "nisNetgroupTriple"
	netgroupMemberAttr = "memberNisNetgroup"
)

var defaultNetgroupAttributes = []string{
	"dn", "cn", "description", netgroupTripleAttr, netgroupMemberAttr,
}

// Netgroup is a nisNetgroup entry.
//
// A netgroup holds two independent things: triples naming hosts and users
// directly, and the names of other netgroups it absorbs. The second makes this a
// graph rather than a list — nothing in the directory forbids a cycle — so
// anything that needs the *effective* membership has to expand it deliberately
// rather than reading Members and stopping there.
type Netgroup struct {
	DN          string `json:"dn"`
	CN          string `json:"cn"`
	Description string `json:"description,omitempty"`

	// Triples that parsed. The stored order is preserved.
	Triples []NetgroupTriple `json:"triples"`

	// Malformed holds the raw values this could not read, verbatim.
	//
	// They are surfaced rather than dropped, and that is the whole point: a
	// triple the app cannot parse is one it must not rewrite either, so a rename
	// has to report it as a leftover instead of silently leaving a stale uid
	// behind. Dropping them would also make a save delete data the operator never
	// asked to touch.
	Malformed []string `json:"malformed,omitempty"`

	// Members names other netgroups absorbed by this one (memberNisNetgroup).
	Members []string `json:"members,omitempty"`
}

// Kind describes what a netgroup is usable for, derived from its triples rather
// than declared.
//
// The schema has no notion of a user netgroup or a host netgroup: both are
// nisNetgroup, and the difference is only which slot people fill in. It matters
// because sudo reads "+name" from sudoUser and from sudoHost, and a netgroup
// used on the wrong side matches nothing at all — silently. Deriving the answer
// keeps the directory portable; declaring it in an attribute would invent
// something no other tool would honour.
type NetgroupKind string

const (
	// NetgroupKindUsers: every triple names a user and no host.
	NetgroupKindUsers NetgroupKind = "users"
	// NetgroupKindHosts: every triple names a host and no user.
	NetgroupKindHosts NetgroupKind = "hosts"
	// NetgroupKindMixed: both kinds of triple are present. Legal, and a warning
	// sign — half of it will be ignored whichever side it is used from.
	NetgroupKindMixed NetgroupKind = "mixed"
	// NetgroupKindEmpty: nothing to go on. A netgroup that only nests others has
	// no kind of its own; ask the expansion instead.
	NetgroupKindEmpty NetgroupKind = "empty"
)

// Kind reports what this netgroup's own triples make it usable for. It looks at
// this entry alone: a netgroup whose content comes entirely through
// memberNisNetgroup answers "empty", which is honest rather than helpful — the
// caller that needs more should expand first.
func (n *Netgroup) Kind() NetgroupKind {
	var users, hosts bool
	for _, triple := range n.Triples {
		// A "-" slot excludes rather than names, so it is not evidence of either
		// kind; an open slot is not evidence either.
		if triple.User != "" && triple.User != NetgroupWildcard {
			users = true
		}
		if triple.Host != "" && triple.Host != NetgroupWildcard {
			hosts = true
		}
	}
	switch {
	case users && hosts:
		return NetgroupKindMixed
	case users:
		return NetgroupKindUsers
	case hosts:
		return NetgroupKindHosts
	default:
		return NetgroupKindEmpty
	}
}

func (c *Client) NetgroupBaseDN() string {
	return fmt.Sprintf("%s,%s", c.config.NetgroupOU, c.config.BaseDN)
}

// NetgroupPage is one page of a netgroup listing.
type NetgroupPage struct {
	Items     []Netgroup
	Total     int
	Truncated bool
}

// SearchNetgroupsPage lists netgroups matching an optional search term, sorted
// and sliced to one page.
//
// The term is applied here rather than by the directory, which is a deliberate
// departure from every other listing in this package. nisNetgroupTriple is
// declared with a SYNTAX and *no* matching rules:
//
//	( 1.3.6.1.1.1.1.14 NAME 'nisNetgroupTriple' DESC 'Netgroup triple'
//	  SYNTAX 1.3.6.1.1.1.0.0 )
//
// so a filter over it cannot be evaluated. OpenLDAP does not reject such a
// filter — it answers "success" and matches nothing, which is the worst of both
// worlds: a search box that looks like it works and silently never finds a
// member. Presence, (nisNetgroupTriple=*), is the only server-side test
// available, because presence needs no matching rule.
//
// Filtering in Go is affordable for the same reason it is for sudo roles: a
// netgroup carries no photo or member expansion, and there are few of them.
// SearchLimited still bounds what is fetched.
func (c *Client) SearchNetgroupsPage(opts ListOptions) (*NetgroupPage, error) {
	entries, truncated, err := c.SearchLimited(c.NetgroupBaseDN(),
		"(objectClass="+netgroupObjectClass+")", defaultNetgroupAttributes)
	if err != nil {
		return nil, err
	}

	netgroups := make([]Netgroup, 0, len(entries))
	for _, entry := range entries {
		netgroup := entryToNetgroup(entry)
		if netgroupMatches(netgroup, opts.Search) {
			netgroups = append(netgroups, netgroup)
		}
	}

	sortBy(netgroups, opts.Desc, func(a, b Netgroup) bool { return foldLess(a.CN, b.CN) })

	return &NetgroupPage{
		Items:     page(netgroups, opts),
		Total:     len(netgroups),
		Truncated: truncated,
	}, nil
}

// netgroupMatches reports whether a netgroup satisfies a free-text term, matching
// what a server-side substring filter would have done over cn and description,
// plus the triples and nested names the directory cannot search.
func netgroupMatches(netgroup Netgroup, term string) bool {
	term = strings.TrimSpace(strings.ToLower(term))
	if term == "" {
		return true
	}
	if strings.Contains(strings.ToLower(netgroup.CN), term) ||
		strings.Contains(strings.ToLower(netgroup.Description), term) {
		return true
	}
	for _, member := range netgroup.Members {
		if strings.Contains(strings.ToLower(member), term) {
			return true
		}
	}
	for _, triple := range netgroup.Triples {
		if strings.Contains(strings.ToLower(triple.String()), term) {
			return true
		}
	}
	for _, raw := range netgroup.Malformed {
		if strings.Contains(strings.ToLower(raw), term) {
			return true
		}
	}
	return false
}

// ListNetgroups returns every netgroup, for the callers that need the whole set
// rather than a page — the expansion and the rename cleanup both do.
func (c *Client) ListNetgroups() ([]Netgroup, error) {
	entries, err := c.Search(c.NetgroupBaseDN(),
		"(objectClass="+netgroupObjectClass+")", defaultNetgroupAttributes)
	if err != nil {
		return nil, err
	}
	netgroups := make([]Netgroup, 0, len(entries))
	for _, entry := range entries {
		netgroups = append(netgroups, entryToNetgroup(entry))
	}
	return netgroups, nil
}

func (c *Client) GetNetgroup(dn string) (*Netgroup, error) {
	entry, err := c.GetEntry(dn, defaultNetgroupAttributes)
	if err != nil {
		return nil, err
	}
	netgroup := entryToNetgroup(entry)
	return &netgroup, nil
}

// GetNetgroupByCN finds a netgroup by name. Netgroups are referenced by name
// everywhere — from memberNisNetgroup, and from sudo as "+name" — so this is the
// lookup most callers actually want.
func (c *Client) GetNetgroupByCN(cn string) (*Netgroup, error) {
	entries, err := c.Search(c.NetgroupBaseDN(),
		fmt.Sprintf("(&(objectClass=%s)(cn=%s))", netgroupObjectClass, ldap.EscapeFilter(cn)),
		defaultNetgroupAttributes)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("netgroup %q not found", cn)
	}
	netgroup := entryToNetgroup(entries[0])
	return &netgroup, nil
}

type CreateNetgroupRequest struct {
	CN          string           `json:"cn"`
	Description string           `json:"description,omitempty"`
	Triples     []NetgroupTriple `json:"triples,omitempty"`
	Members     []string         `json:"members,omitempty"`
}

func (c *Client) CreateNetgroup(req CreateNetgroupRequest) (*Netgroup, error) {
	if strings.TrimSpace(req.CN) == "" {
		return nil, fmt.Errorf("netgroup name is required")
	}

	dn := fmt.Sprintf("cn=%s,%s", ldap.EscapeDN(req.CN), c.NetgroupBaseDN())
	addReq := ldap.NewAddRequest(dn, nil)
	addReq.Attribute("objectClass", []string{"top", netgroupObjectClass})
	addReq.Attribute("cn", []string{req.CN})
	if req.Description != "" {
		addReq.Attribute("description", []string{req.Description})
	}
	if values := formatTriples(req.Triples); len(values) > 0 {
		addReq.Attribute(netgroupTripleAttr, values)
	}
	if len(req.Members) > 0 {
		addReq.Attribute(netgroupMemberAttr, req.Members)
	}

	if err := c.Add(addReq); err != nil {
		return nil, err
	}
	return c.GetNetgroup(dn)
}

// UpdateNetgroupRequest carries only the fields the caller means to change; a nil
// slice leaves the attribute alone, where an empty non-nil slice clears it.
//
// The distinction matters more here than elsewhere: a save that re-sent a
// re-derived triple list would quietly drop whatever this app failed to parse,
// so the handler has to be able to say "I am not touching the triples".
type UpdateNetgroupRequest struct {
	Description *string          `json:"description,omitempty"`
	Triples     []NetgroupTriple `json:"triples,omitempty"`
	Members     []string         `json:"members,omitempty"`
	// Malformed values to preserve verbatim alongside Triples. The caller reads
	// them from the current entry and hands them back unchanged.
	KeepMalformed []string `json:"-"`
}

func (c *Client) UpdateNetgroup(dn string, req UpdateNetgroupRequest) (*Netgroup, error) {
	modReq := ldap.NewModifyRequest(dn, nil)
	changed := false

	// Replace, never Delete: see clearing. The netgroup form posts triples and
	// nested names on every save, so an entry with none of one of them asked the
	// directory to delete an attribute it did not have — which is result code 16,
	// and surfaced as an internal server error on the ordinary act of adding a
	// host to a netgroup that nests nothing.
	if req.Description != nil {
		modReq.Replace("description", clearing(*req.Description))
		changed = true
	}

	if req.Triples != nil || len(req.KeepMalformed) > 0 {
		values := append(formatTriples(req.Triples), req.KeepMalformed...)
		modReq.Replace(netgroupTripleAttr, clearing(values...))
		changed = true
	}

	if req.Members != nil {
		modReq.Replace(netgroupMemberAttr, clearing(req.Members...))
		changed = true
	}

	if changed {
		if err := c.Modify(modReq); err != nil {
			return nil, err
		}
	}
	return c.GetNetgroup(dn)
}

func (c *Client) DeleteNetgroup(dn string) error {
	return c.Delete(dn)
}

// NetgroupsForUser returns the netgroups whose own triples name this user.
//
// Every netgroup is fetched and examined, because the directory cannot help:
// nisNetgroupTriple has no matching rules, so a filter over it is answered with
// "success" and no entries. Narrowing on presence would exclude nothing (every
// netgroup worth examining has triples), so it is not worth the extra clause.
//
// The comparison is slot by slot on the parsed triple, which is the part that
// has to be exact: "jdoe" must not match the host web01 named jdoe, the user
// jdoe-backup, or a domain of that name. Direct membership only — a user reached
// through a nesting parent is not returned here.
func (c *Client) NetgroupsForUser(uid string) ([]Netgroup, error) {
	return c.netgroupsMatching(uid, func(triple NetgroupTriple, name string) bool {
		return triple.NamesUser(name)
	})
}

// NetgroupsForHost is the host-slot counterpart of NetgroupsForUser.
func (c *Client) NetgroupsForHost(host string) ([]Netgroup, error) {
	return c.netgroupsMatching(host, func(triple NetgroupTriple, name string) bool {
		return triple.NamesHost(name)
	})
}

func (c *Client) netgroupsMatching(name string, names func(NetgroupTriple, string) bool) ([]Netgroup, error) {
	if name == "" {
		return nil, nil
	}

	all, err := c.ListNetgroups()
	if err != nil {
		return nil, err
	}

	matches := make([]Netgroup, 0)
	for _, netgroup := range all {
		for _, triple := range netgroup.Triples {
			if names(triple, name) {
				matches = append(matches, netgroup)
				break
			}
		}
	}
	return matches, nil
}

func entryToNetgroup(entry *ldap.Entry) Netgroup {
	netgroup := Netgroup{
		DN:          entry.DN,
		CN:          entry.GetAttributeValue("cn"),
		Description: entry.GetAttributeValue("description"),
		Members:     entry.GetAttributeValues(netgroupMemberAttr),
		Triples:     []NetgroupTriple{},
	}

	for _, raw := range entry.GetAttributeValues(netgroupTripleAttr) {
		triple, err := ParseNetgroupTriple(raw)
		if err != nil {
			// Kept verbatim: see Netgroup.Malformed.
			netgroup.Malformed = append(netgroup.Malformed, raw)
			continue
		}
		netgroup.Triples = append(netgroup.Triples, triple)
	}

	return netgroup
}

func formatTriples(triples []NetgroupTriple) []string {
	values := make([]string, 0, len(triples))
	for _, triple := range triples {
		values = append(values, triple.String())
	}
	return values
}

// renameUserInTriples rewrites every slot that names oldUID, leaving the rest of
// each triple alone.
//
// The uid is a field inside a value here, not a value of its own, so the
// wholesale swap used for memberUid and sudoUser does not apply: replacing the
// stored string would also rewrite a host called jdoe, and a substring
// replacement would corrupt jdoe-backup. Only the user slot, and only when it
// names exactly this uid — an open slot must not be filled in, and a "-" must
// not be turned into a name.
func renameUserInTriples(triples []NetgroupTriple, oldUID, newUID string) ([]NetgroupTriple, bool) {
	out := make([]NetgroupTriple, len(triples))
	changed := false
	for i, triple := range triples {
		if triple.NamesUser(oldUID) {
			triple = triple.WithUser(newUID)
			changed = true
		}
		out[i] = triple
	}
	return out, changed
}

// removeUserFromTriples drops every triple naming this uid.
//
// The whole triple goes, not just the user slot. A triple is one statement —
// "(web01,jdoe,)" says jdoe *on* web01 — so it does not independently assert
// that web01 belongs to the netgroup; a netgroup that wanted the host on its own
// would carry "(web01,,)". Blanking the user slot instead would silently widen
// the entry from "jdoe on web01" to "anyone on web01", which is the opposite of
// what removing a user should do.
func removeUserFromTriples(triples []NetgroupTriple, uid string) ([]NetgroupTriple, bool) {
	out := make([]NetgroupTriple, 0, len(triples))
	for _, triple := range triples {
		if triple.NamesUser(uid) {
			continue
		}
		out = append(out, triple)
	}
	return out, len(out) != len(triples)
}

// saveTriples writes a rewritten triple list back, carrying over the values the
// parser could not read.
//
// Without KeepMalformed a rewrite would delete them: UpdateNetgroup replaces the
// attribute wholesale, and they are not in the parsed list. They are also
// precisely the values this cannot migrate, which is why the caller reports them
// rather than assuming the entry is clean.
func (c *Client) saveTriples(netgroup Netgroup, triples []NetgroupTriple) error {
	_, err := c.UpdateNetgroup(netgroup.DN, UpdateNetgroupRequest{
		Triples:       triples,
		KeepMalformed: netgroup.Malformed,
	})
	return err
}

// NetgroupsNesting returns the netgroups that absorb this one through
// memberNisNetgroup.
//
// Unlike the triples, this attribute does carry equality matching, so the
// directory can answer it.
func (c *Client) NetgroupsNesting(cn string) ([]string, error) {
	if cn == "" {
		return nil, nil
	}
	filter := fmt.Sprintf("(&(objectClass=%s)(%s=%s))",
		netgroupObjectClass, netgroupMemberAttr, ldap.EscapeFilter(cn))
	entries, err := c.Search(c.NetgroupBaseDN(), filter, []string{"cn"})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.GetAttributeValue("cn"))
	}
	sort.Strings(names)
	return names, nil
}
