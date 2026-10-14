package ldap

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

// MissingBaseError reports that a search ran against a base entry the directory
// does not have.
//
// It exists to separate the two things LDAP result code 32 means. Looking up one
// entry that is not there is a 404 and an ordinary event; searching a *base* that
// is not there is a deployment whose configuration and directory disagree, and no
// request against that collection will ever succeed until someone acts. The
// second used to surface as "internal server error", which points at the wrong
// party: the fix is an ldapadd or one line of configuration, not a bug report.
type MissingBaseError struct {
	// Setting names the configuration entry the base came from, e.g.
	// "LDAP_NETGROUP_OU". Empty when the base is not one of the configured ones.
	Setting string
	// Collection is what could not be served, in the words the UI uses.
	Collection string

	err error
}

func (e *MissingBaseError) Error() string {
	return fmt.Sprintf("the %s base entry does not exist in the directory (from %s): %v",
		e.Collection, e.Setting, e.err)
}

func (e *MissingBaseError) Unwrap() error { return e.err }

// BaseDN describes one of the entries the app expects to find.
type BaseDN struct {
	Collection string
	Setting    string
	DN         string
}

// BaseDNs are the entries every collection hangs from, with the setting that
// produced each one so a failure can name what to change.
func (c *Client) BaseDNs() []BaseDN {
	return []BaseDN{
		{Collection: "directory root", Setting: "LDAP_BASE_DN", DN: c.BaseDN()},
		{Collection: "users", Setting: "LDAP_USER_OU", DN: c.UserBaseDN()},
		{Collection: "groups", Setting: "LDAP_GROUP_OU", DN: c.GroupBaseDN()},
		{Collection: "sudo roles", Setting: "LDAP_SUDOERS_OU", DN: c.SudoBaseDN()},
		{Collection: "password policies", Setting: "LDAP_PPOLICY_OU", DN: c.PpolicyBaseDN()},
		{Collection: "netgroups", Setting: "LDAP_NETGROUP_OU", DN: c.NetgroupBaseDN()},
	}
}

// MissingBaseDNs returns the configured bases the directory does not hold.
//
// Called once at startup, because that is when an operator can still act before
// anyone hits the resulting failure — and because the answer changes only when
// somebody edits the directory or the configuration, never between two requests.
// A base that cannot be checked at all (the directory is unreachable) is not
// reported as missing: readiness already covers that, and guessing here would
// send the operator after the wrong problem.
func (c *Client) MissingBaseDNs() []BaseDN {
	var missing []BaseDN
	for _, base := range c.BaseDNs() {
		if base.DN == "" {
			continue
		}
		exists, err := c.entryExists(base.DN)
		if err != nil || exists {
			continue
		}
		missing = append(missing, base)
	}
	return missing
}

// EntryExists reports whether the directory holds an entry at dn. A failure to
// ask is an error, not a "no".
func (c *Client) EntryExists(dn string) (bool, error) {
	return c.entryExists(dn)
}

// entryExists asks for the entry itself, base scope, so the question costs one
// lookup and cannot be answered by something underneath it.
func (c *Client) entryExists(dn string) (bool, error) {
	conn, err := c.connect()
	if err != nil {
		return false, err
	}
	req := ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases,
		1, probeTimeLimitSeconds, false, "(objectClass=*)", []string{"1.1"}, nil)

	result, err := conn.Search(req)
	if err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
			return false, nil
		}
		return false, err
	}
	return len(result.Entries) > 0, nil
}

// asMissingBase turns "no such object" on a configured base into the typed
// error. Anything else — including code 32 on an ordinary entry lookup, which
// callers already turn into a 404 — is returned untouched.
func (c *Client) asMissingBase(baseDN string, err error) error {
	if err == nil || !ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return err
	}
	for _, base := range c.BaseDNs() {
		if base.DN != baseDN {
			continue
		}
		return &MissingBaseError{Setting: base.Setting, Collection: base.Collection, err: err}
	}
	return err
}

// asMissingParent is asMissingBase for a write: the DN handed in is the entry
// being created, and the base that has to exist is its parent.
func (c *Client) asMissingParent(dn string, err error) error {
	if err == nil || !ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return err
	}
	comma := strings.Index(dn, ",")
	if comma < 0 {
		return err
	}
	return c.asMissingBase(dn[comma+1:], err)
}

// AsMissingBase reports whether err came from a search base the directory does
// not have, for callers that need to say so.
func AsMissingBase(err error) (*MissingBaseError, bool) {
	var missing *MissingBaseError
	ok := errors.As(err, &missing)
	return missing, ok
}
