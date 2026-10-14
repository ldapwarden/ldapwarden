package ldap

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
)

// ExportScope is how much of the tree under the base an export covers.
type ExportScope string

const (
	ExportBase     ExportScope = "base"
	ExportOneLevel ExportScope = "one"
	ExportSubtree  ExportScope = "sub"
)

func (s ExportScope) ldapScope() (int, bool) {
	switch s {
	case ExportBase:
		return ldap.ScopeBaseObject, true
	case ExportOneLevel:
		return ldap.ScopeSingleLevel, true
	case ExportSubtree:
		return ldap.ScopeWholeSubtree, true
	}
	return 0, false
}

// ExportOptions selects what ExportLDIF writes.
type ExportOptions struct {
	BaseDN string
	Scope  ExportScope
	// Operational adds every operational attribute ("+"): entryUUID, timestamps,
	// pwdChangedTime, pwdHistory… What slapadd restores, but ldapadd refuses
	// most of them — leave it off for a file meant to be replayed online.
	Operational bool
}

// ExportStats describes a finished export.
type ExportStats struct {
	Entries int `json:"entries"`
	// Orphans were written without their parent having been: a parent the bind
	// identity cannot read, typically. ldapadd will refuse them; they are in the
	// file so their content is not lost.
	Orphans int `json:"orphans"`
}

// exportPageSize is how many entries one paged-results request asks for. Paging
// keeps memory flat and stays under a server's size limit for the identity,
// which a single search would hit on any directory of size.
const exportPageSize = 500

// ppolicyStateAttributes are the operational attributes that carry an account's
// state rather than the server's bookkeeping, and that ldapadd accepts back.
//
// An export without them restores every account unlocked, without its expiry
// and on the default policy: pwdAccountLockedTime, pwdEndTime and
// pwdPolicySubentry are operational, so "*" does not return them. The ones
// slapd maintains itself (pwdChangedTime, pwdHistory, pwdFailureTime) are
// NO-USER-MODIFICATION, and listing them here would make the file unreplayable.
var ppolicyStateAttributes = []string{
	"pwdPolicySubentry", "pwdAccountLockedTime", "pwdStartTime", "pwdEndTime", "pwdReset",
}

// ExportLDIF writes the entries under opts.BaseDN to w as RFC 2849 LDIF, every
// value byte for byte: binary attributes such as jpegPhoto are base64-encoded,
// and so is any value LDIF cannot carry as text.
//
// The output can be replayed with ldapadd. That needs every parent before its
// children, which search order does not promise — back-mdb returns entries in
// creation order, so an entry moved under a newer parent comes first. Such an
// entry is held back until its parent has been written, and only those are: the
// export is streamed page by page, never held in memory as a whole.
//
// An error part-way through leaves w holding a prefix of the export. The file
// ends with a "# end of export" line only when it is complete.
func (c *Client) ExportLDIF(w io.Writer, opts ExportOptions) (ExportStats, error) {
	scope, ok := opts.Scope.ldapScope()
	if !ok {
		return ExportStats{}, fmt.Errorf("unknown export scope %q", opts.Scope)
	}
	conn, err := c.connect()
	if err != nil {
		return ExportStats{}, err
	}

	attributes := append([]string{"*"}, ppolicyStateAttributes...)
	if opts.Operational {
		attributes = []string{"*", "+"}
	}

	out := newLDIFWriter(w)
	out.comment(fmt.Sprintf("ldapwarden export of %s (scope %s, operational attributes %s), %s",
		opts.BaseDN, opts.Scope, map[bool]string{true: "included", false: "omitted"}[opts.Operational],
		time.Now().UTC().Format(time.RFC3339)))
	out.comment("Contains password hashes: store it like any other secret.")
	out.line("version: 1")
	out.line("")

	order := newParentFirst(opts.Scope == ExportSubtree, opts.BaseDN, out.entry)

	paging := ldap.NewControlPaging(exportPageSize)
	request := ldap.NewSearchRequest(opts.BaseDN, scope, ldap.NeverDerefAliases,
		0, searchTimeLimitSeconds(c.timeout()), false, "(objectClass=*)", attributes,
		[]ldap.Control{paging})
	for {
		result, err := conn.Search(request)
		if err != nil {
			if missing := c.asMissingBase(opts.BaseDN, err); missing != err {
				return order.stats(), missing
			}
			return order.stats(), fmt.Errorf("export search: %w", err)
		}
		for _, entry := range result.Entries {
			order.add(entry)
		}
		if out.err != nil {
			return order.stats(), out.err
		}
		control, ok := ldap.FindControl(result.Controls, ldap.ControlTypePaging).(*ldap.ControlPaging)
		if !ok || len(control.Cookie) == 0 {
			break
		}
		paging.SetCookie(control.Cookie)
	}

	if orphans := order.flushOrphans(); orphans > 0 {
		out.comment(fmt.Sprintf("%d entries above were written without their parent, which the export did not return.", orphans))
	}
	stats := order.stats()
	out.comment(fmt.Sprintf("end of export: %d entries", stats.Entries))
	return stats, out.flush()
}

// parentFirst emits entries so that none precedes its parent, holding back only
// the ones that arrive early.
type parentFirst struct {
	subtree  bool
	baseKey  string
	emit     func(*ldap.Entry)
	written  map[string]bool
	waiting  map[string][]*ldap.Entry // by parent key
	count    int
	orphaned int
}

func newParentFirst(subtree bool, baseDN string, emit func(*ldap.Entry)) *parentFirst {
	return &parentFirst{
		subtree: subtree,
		baseKey: dnKey(baseDN),
		emit:    emit,
		written: map[string]bool{},
		waiting: map[string][]*ldap.Entry{},
	}
}

func (p *parentFirst) add(entry *ldap.Entry) {
	key := dnKey(entry.DN)
	// Only a subtree export contains parents to wait for; the base entry's own
	// parent is outside the export by definition.
	if p.subtree && key != p.baseKey {
		if parent := parentKey(entry.DN); !p.written[parent] {
			p.waiting[parent] = append(p.waiting[parent], entry)
			return
		}
	}
	p.write(entry, key)
}

func (p *parentFirst) write(entry *ldap.Entry, key string) {
	p.emit(entry)
	p.count++
	p.written[key] = true
	children := p.waiting[key]
	delete(p.waiting, key)
	for _, child := range children {
		p.write(child, dnKey(child.DN))
	}
}

// flushOrphans writes whatever is still waiting: entries whose parent never
// came. Returns how many.
func (p *parentFirst) flushOrphans() int {
	before := p.count
	for len(p.waiting) > 0 {
		for parent, children := range p.waiting {
			delete(p.waiting, parent)
			for _, child := range children {
				p.write(child, dnKey(child.DN))
			}
		}
	}
	p.orphaned = p.count - before
	return p.orphaned
}

func (p *parentFirst) stats() ExportStats {
	return ExportStats{Entries: p.count, Orphans: p.orphaned}
}

// dnKey normalises a DN for comparison: attribute types and values are
// case-folded and spacing is dropped, the way the directory compares the names
// of the attributes these RDNs use. A DN that does not parse is compared as text.
func dnKey(dn string) string {
	parsed, err := ldap.ParseDN(dn)
	if err != nil {
		return strings.ToLower(dn)
	}
	return rdnsKey(parsed.RDNs)
}

func parentKey(dn string) string {
	parsed, err := ldap.ParseDN(dn)
	if err != nil || len(parsed.RDNs) == 0 {
		return ""
	}
	return rdnsKey(parsed.RDNs[1:])
}

func rdnsKey(rdns []*ldap.RelativeDN) string {
	parts := make([]string, len(rdns))
	for i, rdn := range rdns {
		attrs := make([]string, len(rdn.Attributes))
		for j, attr := range rdn.Attributes {
			attrs[j] = strings.ToLower(attr.Type) + "=" + strings.ToLower(attr.Value)
		}
		parts[i] = strings.Join(attrs, "+")
	}
	return strings.Join(parts, ",")
}

// ldifWriter writes RFC 2849 LDIF. The first write error is kept and every
// later write is skipped, so the caller checks once.
type ldifWriter struct {
	w   *bufio.Writer
	err error
}

func newLDIFWriter(w io.Writer) *ldifWriter {
	return &ldifWriter{w: bufio.NewWriterSize(w, 64*1024)}
}

// ldifLineWidth is where lines are folded. RFC 2849 allows any length but
// recommends 76, which is what ldapsearch writes.
const ldifLineWidth = 76

func (l *ldifWriter) line(s string) {
	if l.err != nil {
		return
	}
	// Folding splits bytes, which is safe because every line is ASCII: a value
	// that is not is base64-encoded first.
	for len(s) > ldifLineWidth {
		if _, l.err = l.w.WriteString(s[:ldifLineWidth] + "\n "); l.err != nil {
			return
		}
		s = s[ldifLineWidth:]
	}
	_, l.err = l.w.WriteString(s + "\n")
}

func (l *ldifWriter) comment(s string) {
	l.line("# " + s)
}

func (l *ldifWriter) entry(entry *ldap.Entry) {
	l.line(attrValue("dn", []byte(entry.DN)))
	for _, attr := range entry.Attributes {
		for _, value := range attr.ByteValues {
			l.line(attrValue(attr.Name, value))
		}
	}
	l.line("")
}

func (l *ldifWriter) flush() error {
	if l.err != nil {
		return l.err
	}
	return l.w.Flush()
}

// attrValue renders one "name: value" line, base64-encoded ("name:: …") when
// the value is not a SAFE-STRING.
func attrValue(name string, value []byte) string {
	if len(value) == 0 {
		return name + ":"
	}
	if isSafeString(value) {
		return name + ": " + string(value)
	}
	return name + ":: " + base64.StdEncoding.EncodeToString(value)
}

// isSafeString is RFC 2849's SAFE-STRING: 7-bit, no NUL, CR or LF, and not
// starting with a space, a colon or "<". A trailing space is excluded as well,
// as the RFC recommends — editors and copy-paste strip them.
func isSafeString(value []byte) bool {
	switch value[0] {
	case ' ', ':', '<':
		return false
	}
	if value[len(value)-1] == ' ' {
		return false
	}
	for _, b := range value {
		if b == 0 || b == '\n' || b == '\r' || b > 0x7f {
			return false
		}
	}
	return true
}

// ErrExportScope is returned for a scope ExportLDIF does not know.
var ErrExportScope = errors.New("scope must be one of base, one, sub")

// ParseExportScope reads a scope from a request, defaulting to the subtree.
func ParseExportScope(s string) (ExportScope, error) {
	if s == "" {
		return ExportSubtree, nil
	}
	scope := ExportScope(s)
	if _, ok := scope.ldapScope(); !ok {
		return "", ErrExportScope
	}
	return scope, nil
}
