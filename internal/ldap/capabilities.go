package ldap

import (
	"strings"

	"github.com/go-ldap/ldap/v3"
)

// Capabilities records what the connected directory can actually enforce, so the
// UI can offer only the features that will work rather than writing attributes
// the server ignores.
type Capabilities struct {
	// ScheduledExpiry reports that the ppolicy overlay provides pwdEndTime
	// (OpenLDAP 2.5 and later). That attribute is the only portable way to say
	// "this account stops working on date X" and have the *directory* enforce it:
	//
	//   - a future value does not block anything, so scheduling is safe;
	//   - a past value refuses the bind, and keeps refusing it, because
	//     pwdEndTime does not interact with pwdLockoutDuration.
	//
	// pwdAccountLockedTime cannot express this: OpenLDAP treats it as the instant
	// a lockout began, so a future value disables the account immediately and a
	// past one stops having any effect once pwdLockoutDuration has elapsed.
	ScheduledExpiry bool
}

// pwdEndTimeAttr is the attribute whose presence in the subschema decides
// ScheduledExpiry.
const pwdEndTimeAttr = "pwdEndTime"

// Capabilities returns the cached capability set, probing the directory on first
// use. A probe failure is not fatal: it yields the conservative answer (no
// scheduled expiry), because offering a feature the server will silently ignore
// is worse than hiding one it might support.
func (c *Client) Capabilities() Capabilities {
	c.capMu.Lock()
	defer c.capMu.Unlock()

	if c.capProbed {
		return c.caps
	}

	c.caps = Capabilities{ScheduledExpiry: c.hasAttributeType(pwdEndTimeAttr)}
	c.capProbed = true
	return c.caps
}

// RefreshCapabilities forgets the cached probe, so the next Capabilities call
// asks the directory again. Called when the schema cache is refreshed.
func (c *Client) RefreshCapabilities() {
	c.capMu.Lock()
	c.capProbed = false
	c.capMu.Unlock()
}

// hasAttributeType reports whether the directory's subschema declares an
// attribute type of this name.
//
// It matches on the quoted name rather than parsing every definition: the
// subschema is one multi-megabyte value, the answer is a yes/no, and a substring
// check on "'pwdEndTime'" cannot collide — attribute names are unique and always
// quoted in a NAME clause.
func (c *Client) hasAttributeType(name string) bool {
	conn, err := c.connect()
	if err != nil {
		return false
	}

	req := ldap.NewSearchRequest(
		"cn=Subschema",
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		0, 0, false,
		"(objectClass=*)",
		[]string{"attributeTypes"},
		nil,
	)

	result, err := conn.Search(req)
	if err != nil {
		// Some servers expose the subschema under the root DSE instead.
		req.BaseDN = ""
		result, err = conn.Search(req)
		if err != nil || len(result.Entries) == 0 {
			return false
		}
	}
	if len(result.Entries) == 0 {
		return false
	}

	needle := "'" + name + "'"
	for _, definition := range result.Entries[0].GetAttributeValues("attributeTypes") {
		if strings.Contains(definition, needle) {
			return true
		}
	}
	return false
}
