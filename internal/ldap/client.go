package ldap

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/ldapwarden/ldapwarden/internal/config"
)

// defaultLDAPTimeout applies when the configuration carries none, so a client
// built directly in a test still cannot wait for ever.
const defaultLDAPTimeout = 20 * time.Second

type Client struct {
	config config.LDAPConfig
	mu     sync.Mutex
	conn   *ldap.Conn

	// Capability probe result. The subschema does not change while slapd runs,
	// short of an administrator loading a module, so it is cached until
	// RefreshCapabilities is called.
	capMu     sync.Mutex
	capProbed bool
	caps      Capabilities
	// capProbing is non-nil while a probe is running, and closed when it
	// finishes. It lets concurrent callers wait on that one probe instead of
	// serialising behind capMu to each run their own — and, more to the point,
	// keeps the mutex from being held across network I/O.
	capProbing chan struct{}
	// verdict is what a policy self-test observed, if one has been run. It fills
	// in for the passive enforcement probe, which normally cannot tell.
	verdict Enforcement
}

// dial opens a connection to the directory, encrypted as configured, with the
// operation timeout applied. It does not bind.
//
// Both the shared service connection and the throwaway one a user's sign-in is
// checked on go through here. They used to carry a copy each, and the sign-in
// copy had neither the dial timeout nor the per-request deadline: a directory
// that stopped answering hung the login handler for good.
func (c *Client) dial() (*ldap.Conn, error) {
	tlsConfig, err := TLSConfig(c.config)
	if err != nil {
		return nil, err
	}

	// A dial that cannot complete has to give up on its own: the handlers hold no
	// context to cancel it with.
	dialer := ldap.DialWithDialer(&net.Dialer{Timeout: c.timeout()})
	ldaps := strings.HasPrefix(strings.ToLower(c.config.URL), "ldaps://")

	var conn *ldap.Conn
	switch c.config.TLSMode {
	case "ssl":
		// LDAPS - TLS from the start (typically port 636). go-ldap only does that
		// for the ldaps:// scheme; on ldap:// it would connect in clear.
		if !ldaps {
			return nil, fmt.Errorf("%w: ssl needs an ldaps:// URL", errTLSSchemeMismatch)
		}
		conn, err = ldap.DialURL(c.config.URL, dialer, ldap.DialWithTLSConfig(tlsConfig))
	case "starttls":
		// StartTLS - connect plain, then upgrade to TLS
		if ldaps {
			return nil, fmt.Errorf("%w: starttls needs an ldap:// URL", errTLSSchemeMismatch)
		}
		conn, err = ldap.DialURL(c.config.URL, dialer)
		if err == nil {
			conn.SetTimeout(c.timeout())
			if err = conn.StartTLS(tlsConfig); err != nil {
				_ = conn.Close()
				return nil, fmt.Errorf("StartTLS: %w%s", err, describeTLS(c.config))
			}
		}
	default:
		// "none" or unset - plain LDAP, or TLS when the URL says ldaps://
		conn, err = ldap.DialURL(c.config.URL, dialer, ldap.DialWithTLSConfig(tlsConfig))
	}
	if err != nil {
		return nil, fmt.Errorf("dial LDAP server: %w%s", err, describeTLS(c.config))
	}

	// Bounds every request made on this connection from here on, bind included.
	// go-ldap sets no deadline by default, so a response that never arrives — a
	// connection a firewall dropped without a FIN, a server wedged mid-search —
	// blocks the calling goroutine for the lifetime of the process, holding
	// whatever lock it took on the way in.
	conn.SetTimeout(c.timeout())
	return conn, nil
}

func NewClient(cfg config.LDAPConfig) *Client {
	return &Client{
		config: cfg,
	}
}

func (c *Client) connect() (*ldap.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		if err := c.conn.Bind(c.config.BindDN, c.config.BindPass); err == nil {
			return c.conn, nil
		}
		_ = c.conn.Close()
	}

	conn, err := c.dial()
	if err != nil {
		return nil, err
	}

	if err := conn.Bind(c.config.BindDN, c.config.BindPass); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("bind to LDAP server: %w", err)
	}

	c.conn = conn
	return conn, nil
}

// searchTimeLimitSeconds is the server-side time limit for a search, expressed in
// the whole seconds the protocol allows.
//
// Deliberately shorter than the connection deadline: the server giving up and
// saying so is an answer, while the client's deadline expiring is a dead
// connection and no explanation. Never zero — that is the protocol's "no limit",
// which is what this exists to avoid.
func searchTimeLimitSeconds(timeout time.Duration) int {
	seconds := int(timeout.Seconds()) - 1
	if seconds < 1 {
		return 1
	}
	return seconds
}

// timeout is the bound applied to every LDAP operation. Zero or negative
// disables it, which is go-ldap's own default and never what a served request
// wants; the config default is what normally applies.
func (c *Client) timeout() time.Duration {
	if c.config.Timeout <= 0 {
		return defaultLDAPTimeout
	}
	return c.config.Timeout
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

func (c *Client) Bind(dn, password string) error {
	conn, err := c.connect()
	if err != nil {
		return err
	}
	_ = conn

	// A separate connection for the user's bind, so the shared one stays bound
	// as the service account.
	testConn, err := c.dial()
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	defer func() { _ = testConn.Close() }()

	if err := testConn.Bind(dn, password); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	return nil
}

func (c *Client) Search(baseDN, filter string, attributes []string) ([]*ldap.Entry, error) {
	conn, err := c.connect()
	if err != nil {
		return nil, err
	}

	searchRequest := ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		// No size limit — the callers that read a whole collection need all of it,
		// and a truncated read here would produce confidently wrong answers rather
		// than a slow one (a netgroup missing from the set reads as a dangling
		// reference). A *time* limit, though, costs nothing and bounds the wait:
		// without it the only stop was the connection deadline, so a directory
		// that answered slowly left the request hanging with nothing to report.
		0, searchTimeLimitSeconds(c.timeout()), false,
		filter,
		attributes,
		nil,
	)

	result, err := conn.Search(searchRequest)
	if err != nil {
		// A base the directory does not hold is a configuration mismatch, not a
		// failed search; it is reported as such so the caller can say which
		// setting to look at. See MissingBaseError.
		if missing := c.asMissingBase(baseDN, err); missing != err {
			return nil, missing
		}
		return nil, fmt.Errorf("search LDAP: %w", err)
	}

	return result.Entries, nil
}

// SearchLimited runs a subtree search bounded by the configured search size
// limit. When more matching entries exist than the limit allows, the first N
// entries are returned with truncated=true instead of an error, so callers can
// surface a "refine your search" hint. A limit of 0 means no bound.
func (c *Client) SearchLimited(baseDN, filter string, attributes []string) (entries []*ldap.Entry, truncated bool, err error) {
	conn, err := c.connect()
	if err != nil {
		return nil, false, err
	}

	searchRequest := ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		c.config.SearchSizeLimit, 0, false,
		filter,
		attributes,
		nil,
	)
	// Also stop client-side once the limit is reached, in case the server
	// does not enforce the requested SizeLimit (e.g. when bound as rootdn).
	searchRequest.EnforceSizeLimit = c.config.SearchSizeLimit > 0

	result, err := conn.Search(searchRequest)
	if err != nil {
		// On size-limit overflow go-ldap still returns the partial result; we
		// treat that as a successful, truncated search rather than an error.
		if result != nil && (errors.Is(err, ldap.ErrSizeLimitExceeded) || ldap.IsErrorWithCode(err, ldap.LDAPResultSizeLimitExceeded)) {
			return result.Entries, true, nil
		}
		if missing := c.asMissingBase(baseDN, err); missing != err {
			return nil, false, missing
		}
		return nil, false, fmt.Errorf("search LDAP: %w", err)
	}

	return result.Entries, false, nil
}

// substringFilter builds an OR of case-insensitive substring matches across
// the given attributes for an (untrusted) search term, e.g.
// "(|(uid=*term*)(cn=*term*))". Returns "" when the term is empty so callers
// can fall back to their base filter.
func substringFilter(term string, attrs ...string) string {
	term = strings.TrimSpace(term)
	if term == "" {
		return ""
	}
	esc := ldap.EscapeFilter(term)
	var b strings.Builder
	b.WriteString("(|")
	for _, a := range attrs {
		fmt.Fprintf(&b, "(%s=*%s*)", a, esc)
	}
	b.WriteString(")")
	return b.String()
}

// andFilter combines a base filter with an optional extra clause:
// andFilter("(objectClass=x)", "(cn=*y*)") -> "(&(objectClass=x)(cn=*y*))".
func andFilter(base, extra string) string {
	if extra == "" {
		return base
	}
	return "(&" + base + extra + ")"
}

func (c *Client) GetEntry(dn string, attributes []string) (*ldap.Entry, error) {
	conn, err := c.connect()
	if err != nil {
		return nil, err
	}

	searchRequest := ldap.NewSearchRequest(
		dn,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		1, 0, false,
		"(objectClass=*)",
		attributes,
		nil,
	)

	result, err := conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("get entry: %w", err)
	}

	if len(result.Entries) == 0 {
		return nil, fmt.Errorf("entry not found: %s", dn)
	}

	return result.Entries[0], nil
}

// noAttributes is RFC 4511's "return no attributes" request. A count needs to
// know how many entries match, not what is in them.
const noAttributes = "1.1"

// GeneralizedTime formats an instant the way the directory stores one, for use in
// a search filter. Ordering comparisons on generalizedTime are standard, so
// "(pwdEndTime<=...)" is a filter the server can answer.
func GeneralizedTime(t time.Time) string {
	return t.UTC().Format(generalizedTimeLayout)
}

// CountEntries counts the entries matching a filter without fetching them.
//
// Deliberately *not* bounded by SearchSizeLimit: that limit exists to keep a list
// view's payload small, and applying it to a count made the dashboard cap itself
// at 500 and then warn the operator about its own cap.
//
// Asking for no attributes keeps this affordable, though the cost does grow with
// the directory: measured on 706 entries it moved 55 kB where the old capped-and-
// wrong version moved 82 kB, but the old one was bounded at 500 entries and this
// is not. Index the attributes these filters name if the dashboard gets slow —
// pwdAccountLockedTime and pwdEndTime are unindexed in a default configuration.
//
// The returned bool reports that the *server* stopped early, which is a real
// condition an operator can act on (olcSizeLimit for the bind identity) rather
// than a self-inflicted one. The DNs are held in memory only for the duration of
// the call; at 100k entries that is a few MB, against hundreds for full entries.
func (c *Client) CountEntries(baseDN, filter string) (count int, serverLimited bool, err error) {
	conn, err := c.connect()
	if err != nil {
		return 0, false, err
	}

	searchRequest := ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0, 0, false,
		filter,
		[]string{noAttributes},
		nil,
	)

	result, err := conn.Search(searchRequest)
	if err != nil {
		// slapd returns a partial result set alongside any of these, and all three
		// are limits an operator sets deliberately — often precisely to stop the
		// unindexed scans these count filters perform. Treating them as failures
		// would turn "the answer is a lower bound" into "there is no answer".
		if result != nil && (errors.Is(err, ldap.ErrSizeLimitExceeded) ||
			ldap.IsErrorWithCode(err, ldap.LDAPResultSizeLimitExceeded) ||
			ldap.IsErrorWithCode(err, ldap.LDAPResultTimeLimitExceeded) ||
			ldap.IsErrorWithCode(err, ldap.LDAPResultAdminLimitExceeded)) {
			return len(result.Entries), true, nil
		}
		return 0, false, fmt.Errorf("count entries: %w", err)
	}
	return len(result.Entries), false, nil
}

// Ping reports whether the directory answers, for the readiness probe. It reads
// the base entry rather than only binding: a bind proves the credentials work, this
// also proves the configured base DN exists and is readable, which is the other
// half of "the app can do its job".
func (c *Client) Ping() error {
	if _, err := c.GetEntry(c.config.BaseDN, []string{noAttributes}); err != nil {
		return err
	}
	return nil
}

func (c *Client) Add(entry *ldap.AddRequest) error {
	conn, err := c.connect()
	if err != nil {
		return err
	}

	// "No such object" on an add is about the *parent*: the entry being created
	// has nowhere to go. Same configuration mismatch as a search against a
	// missing base, and worth the same answer — otherwise creating the first
	// netgroup on a directory without an ou=netgroups reports an internal error.
	return c.asMissingParent(entry.DN, conn.Add(entry))
}

func (c *Client) Modify(request *ldap.ModifyRequest) error {
	conn, err := c.connect()
	if err != nil {
		return err
	}

	return conn.Modify(request)
}

// PasswordModify issues an RFC 3062 PasswordModify extended operation against
// dn. Unlike a plain Modify on the userPassword attribute, this lets the
// server's password-policy overlay hash the value and run pre-set hooks
// (history, quality checks). The connection is bound as the configured
// BindDN, so oldPassword is empty — the server allows administrative resets
// in that mode.
func (c *Client) PasswordModify(dn, newPassword string) error {
	conn, err := c.connect()
	if err != nil {
		return err
	}
	req := ldap.NewPasswordModifyRequest(dn, "", newPassword)
	_, err = conn.PasswordModify(req)
	return err
}

func (c *Client) Delete(dn string) error {
	conn, err := c.connect()
	if err != nil {
		return err
	}

	delRequest := ldap.NewDelRequest(dn, nil)
	return conn.Del(delRequest)
}

// ModifyDN renames an entry to newRDN (e.g. "uid=alice") under the same parent
// (newSuperior is left empty). With deleteOldRDN true the old RDN attribute
// value is dropped, so the RDN attribute (uid/cn) is updated to match the new
// name. Used to rename a user, whose uid is its RDN.
func (c *Client) ModifyDN(dn, newRDN string, deleteOldRDN bool) error {
	conn, err := c.connect()
	if err != nil {
		return err
	}
	req := ldap.NewModifyDNRequest(dn, newRDN, deleteOldRDN, "")
	return conn.ModifyDN(req)
}

func (c *Client) BaseDN() string {
	return c.config.BaseDN
}

func (c *Client) UserBaseDN() string {
	return fmt.Sprintf("%s,%s", c.config.UserOU, c.config.BaseDN)
}

func (c *Client) GroupBaseDN() string {
	return fmt.Sprintf("%s,%s", c.config.GroupOU, c.config.BaseDN)
}

func (c *Client) MinUID() int {
	return c.config.MinUID
}

func (c *Client) MinGID() int {
	return c.config.MinGID
}

// NextUID finds the next available UID starting from MinUID
func (c *Client) NextUID() (int, error) {
	entries, err := c.Search(c.UserBaseDN(), "(objectClass=posixAccount)", []string{"uidNumber"})
	if err != nil {
		return 0, fmt.Errorf("search for UIDs: %w", err)
	}

	usedUIDs := make(map[int]bool)
	for _, entry := range entries {
		if uidStr := entry.GetAttributeValue("uidNumber"); uidStr != "" {
			if uid, err := strconv.Atoi(uidStr); err == nil {
				usedUIDs[uid] = true
			}
		}
	}

	// Find the next available UID starting from MinUID
	nextUID := c.config.MinUID
	for usedUIDs[nextUID] {
		nextUID++
	}

	return nextUID, nil
}

// NextGID finds the next available GID starting from MinGID
func (c *Client) NextGID() (int, error) {
	entries, err := c.Search(c.GroupBaseDN(), "(objectClass=posixGroup)", []string{"gidNumber"})
	if err != nil {
		return 0, fmt.Errorf("search for GIDs: %w", err)
	}

	usedGIDs := make(map[int]bool)
	for _, entry := range entries {
		if gidStr := entry.GetAttributeValue("gidNumber"); gidStr != "" {
			if gid, err := strconv.Atoi(gidStr); err == nil {
				usedGIDs[gid] = true
			}
		}
	}

	// Find the next available GID starting from MinGID
	nextGID := c.config.MinGID
	for usedGIDs[nextGID] {
		nextGID++
	}

	return nextGID, nil
}

// clearing is the value set for a Modify that removes an attribute.
//
// LDAP has two ways to say "this attribute should be gone", and only one of them
// is safe to send without knowing whether it is there. A delete of an absent
// attribute is an error — noSuchAttribute, result code 16 — while a replace with
// no values removes it if present and does nothing if not (RFC 4511 §4.6).
//
// The difference matters because a form sends every field it manages, including
// the empty ones: saving a netgroup that had no nested netgroups, or a group with
// no description, asked the directory to delete an attribute that was never
// there and came back as an internal server error. Passing the values through
// this helper makes the caller's intent — "make it this, which may be nothing" —
// the thing that is sent.
func clearing(values ...string) []string {
	kept := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			kept = append(kept, value)
		}
	}
	return kept
}
