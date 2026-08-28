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
	//
	// This answers "does the server know the attribute", NOT "will anything be
	// enforced" — see PolicyEnforcement, which is a different question with a
	// different answer.
	ScheduledExpiry bool

	// PolicyEnforcement reports whether a password policy is actually in effect.
	//
	// It matters because ppolicy enforces nothing unless a policy applies to the
	// entry, and the attributes stay writable regardless: with the overlay loaded
	// but no olcPPolicyDefault and no pwdPolicySubentry, locking an account and
	// expiring it both succeed and both do nothing, while the UI reports the
	// account as locked or expired. Measured, not assumed — see the constants.
	PolicyEnforcement Enforcement

	// Netgroups reports that the directory declares nisNetgroup, i.e. that
	// nis.schema is loaded. Unlike the two above this is a plain schema question
	// with a reliable answer — a netgroup entry either can or cannot be created.
	//
	// It says nothing about whether sudo will honour a "+netgroup" grant: that is
	// decided on each client by NSS, ldap.conf's netgroup_base or SSSD, none of
	// which is visible from here. The UI has to say so rather than imply that
	// writing the grant is the end of the job.
	Netgroups bool
}

// Enforcement is a three-valued answer, because the honest answer is usually
// "cannot tell from here".
//
// Every signal readable with an ordinary bind identity is identical whether or
// not a policy applies: cn=config is hidden, back-monitor is usually absent, the
// server does not synthesise the effective pwdPolicySubentry, the ppolicy control
// is advertised by the module rather than by a policy, and pwdChangedTime is
// stamped either way. pwdFailureTime does differ — but only because it tracks
// pwdMaxFailure, so a policy with pwdLockout FALSE looks exactly like no policy
// while still enforcing pwdEndTime. Guessing from it would warn about a directory
// that works.
//
// So: only ever conclude EnforcementNotEnforced from evidence, never from the
// absence of evidence.
type Enforcement string

const (
	// EnforcementEnforced: a policy was seen to apply.
	EnforcementEnforced Enforcement = "enforced"
	// EnforcementNotEnforced: the overlay was read and no default policy is set,
	// or a self-test proved a lock had no effect.
	EnforcementNotEnforced Enforcement = "not-enforced"
	// EnforcementUnknown: cn=config is not readable with this bind identity, which
	// is the normal case. Run the self-test for a definitive answer.
	EnforcementUnknown Enforcement = "unknown"
)

// pwdEndTimeAttr is the attribute whose presence in the subschema decides
// ScheduledExpiry.
const pwdEndTimeAttr = "pwdEndTime"

// ppolicyConfigClass is the objectClass of the overlay's configuration entry.
const ppolicyConfigClass = "olcPPolicyConfig"

// probeTimeLimitSeconds is the server-side time limit on the two probe searches.
const probeTimeLimitSeconds = 10

// Capabilities returns the cached capability set, probing the directory on first
// use. A probe failure is not fatal: it yields the conservative answer (no
// scheduled expiry), because offering a feature the server will silently ignore
// is worse than hiding one it might support.
//
// The probe runs without capMu held, and concurrent callers wait on the one
// probe already in flight instead of queueing behind the mutex to run their own.
// Holding the lock across two network searches meant a directory that stopped
// answering did not merely slow this call down: it wedged every later caller for
// the lifetime of the process, since the handlers have no context and the
// goroutine that took the lock never returned to release it. In practice that
// took out /api/overview and /api/admin/config — everything that asks what the
// directory supports — while the parts of the app backed by Postgres went on
// working, which made it look like anything but a lock.
func (c *Client) Capabilities() Capabilities {
	c.capMu.Lock()
	for {
		if c.capProbed {
			caps := c.caps
			c.capMu.Unlock()
			return caps
		}
		if c.capProbing == nil {
			break
		}
		// Someone else is already asking; wait for their answer rather than
		// starting a second probe behind theirs.
		inflight := c.capProbing
		c.capMu.Unlock()
		<-inflight
		c.capMu.Lock()
	}

	inflight := make(chan struct{})
	c.capProbing = inflight
	c.capMu.Unlock()

	// Both searches are bounded by the connection deadline and by
	// probeTimeLimitSeconds, so this cannot outlive them.
	enforcement := c.probePolicyEnforcement()
	scheduledExpiry := c.hasAttributeType(pwdEndTimeAttr)
	netgroups := c.hasObjectClass(netgroupObjectClass)

	c.capMu.Lock()
	// A self-test actually proved something; the passive probe usually cannot. Let
	// the proof win, but never let it override a config read that disagrees — that
	// would hide a policy having been removed since.
	if enforcement == EnforcementUnknown && c.verdict != "" {
		enforcement = c.verdict
	}
	c.caps = Capabilities{
		ScheduledExpiry:   scheduledExpiry,
		PolicyEnforcement: enforcement,
		Netgroups:         netgroups,
	}
	c.capProbed = true
	c.capProbing = nil
	caps := c.caps
	c.capMu.Unlock()

	// Releases anyone who arrived while this probe was running.
	close(inflight)
	return caps
}

// RecordEnforcementVerdict stores what a self-test observed, so the answer stops
// being "unknown" once someone has checked.
//
// In memory only, and deliberately: it describes the directory as it was a moment
// ago, and a restart should go back to measuring rather than trusting a verdict
// nobody can date. Re-run the self-test after changing the policy configuration.
func (c *Client) RecordEnforcementVerdict(verdict Enforcement) {
	c.capMu.Lock()
	c.verdict = verdict
	c.capProbed = false
	c.capMu.Unlock()
}

// probePolicyEnforcement reads the overlay's configuration entry, which settles
// the question outright — when the bind identity is allowed to see it.
//
// Most deployments bind as the database rootdn, which cannot read cn=config:
// slapd answers "no such object" rather than "insufficient access", so a refusal
// is indistinguishable from a directory with no overlay. That ambiguity is
// exactly why the negative case is Unknown and not NotEnforced.
func (c *Client) probePolicyEnforcement() Enforcement {
	conn, err := c.connect()
	if err != nil {
		return EnforcementUnknown
	}

	req := ldap.NewSearchRequest(
		"cn=config",
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		// A server-side time limit as well as the connection deadline: this one
		// asks the directory to stop working, where the deadline only stops us
		// waiting for it. A whole-subtree search on cn=config is the most
		// exposed request the app makes — usually refused outright, occasionally
		// answered by a config tree of unknown size.
		0, probeTimeLimitSeconds, false,
		"(objectClass="+ppolicyConfigClass+")",
		[]string{"olcPPolicyDefault"},
		nil,
	)

	result, err := conn.Search(req)
	if err != nil || len(result.Entries) == 0 {
		return EnforcementUnknown
	}

	// A directory may carry the overlay on several databases. One default policy
	// anywhere is enough to know enforcement happens somewhere; saying which
	// database would need the entry's DN parsed against the user base, which is
	// more precision than this answer is worth.
	for _, entry := range result.Entries {
		if entry.GetAttributeValue("olcPPolicyDefault") != "" {
			return EnforcementEnforced
		}
	}
	return EnforcementNotEnforced
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
func (c *Client) hasAttributeType(name string) bool {
	return c.schemaDeclares("attributeTypes", name)
}

// hasObjectClass reports whether the directory's subschema declares an object
// class of this name — the question behind Netgroups, since nisNetgroup comes
// from nis.schema and a directory is free not to load it.
func (c *Client) hasObjectClass(name string) bool {
	return c.schemaDeclares("objectClasses", name)
}

// schemaDeclares reports whether the subschema's schemaAttr ("attributeTypes" or
// "objectClasses") declares something of this name.
//
// It matches on the quoted name rather than parsing every definition: the
// subschema is one multi-megabyte value, the answer is a yes/no, and a substring
// check on "'pwdEndTime'" cannot collide — schema names are unique and always
// quoted in a NAME clause.
func (c *Client) schemaDeclares(schemaAttr, name string) bool {
	conn, err := c.connect()
	if err != nil {
		return false
	}

	req := ldap.NewSearchRequest(
		"cn=Subschema",
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		// The subschema is a single multi-megabyte value; bound the server's work
		// as well as our wait for it.
		0, probeTimeLimitSeconds, false,
		"(objectClass=*)",
		[]string{schemaAttr},
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
	for _, definition := range result.Entries[0].GetAttributeValues(schemaAttr) {
		if strings.Contains(definition, needle) {
			return true
		}
	}
	return false
}
