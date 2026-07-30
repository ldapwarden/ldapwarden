package ldap

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/go-ldap/ldap/v3"
)

// PolicySelfTestUID is the uid of the entry the self-test creates. Fixed and
// obvious rather than random: it shows up in the audit log and in the directory
// for the second or so it exists, and an operator who finds a leftover should be
// able to tell what made it.
const PolicySelfTestUID = "ldapwarden-policy-selftest"

// ErrSelfTestEntryExists is returned when the probe DN is already taken. The
// self-test never writes over an entry it did not create.
var ErrSelfTestEntryExists = errors.New("the self-test entry already exists: remove it, or check whether another run is in progress")

// PolicySelfTest is the outcome of TestPolicyEnforcement.
type PolicySelfTest struct {
	// Result is the verdict: enforced, not-enforced, or unknown when the test
	// could not reach a conclusion (which is not the same as "not enforced").
	Result Enforcement `json:"result"`
	// ProbeDN is the entry that was created and removed.
	ProbeDN string `json:"probeDn"`
	// Steps records what the test did, in order, so the verdict can be checked
	// rather than trusted.
	Steps []string `json:"steps"`
	// CleanedUp is false when the probe entry could not be removed — the one
	// failure an operator has to act on.
	CleanedUp bool `json:"cleanedUp"`
	// Detail explains an inconclusive result.
	Detail string `json:"detail,omitempty"`
}

// TestPolicyEnforcement answers, definitively, whether the directory enforces a
// password policy on entries in the user OU.
//
// Nothing readable settles this — every passive signal is identical whether or
// not a policy applies (see Enforcement). The only proof is behavioural: create
// an entry, lock it, and see whether the directory refuses the bind. That means
// writing to the directory, so this is an explicit operator action rather than a
// probe on startup.
//
// The verdict is about "a newly created entry in the user OU". A policy attached
// per-entry through pwdPolicySubentry applies to those entries and says nothing
// about this one, and vice versa.
func (c *Client) TestPolicyEnforcement() (*PolicySelfTest, error) {
	dn := fmt.Sprintf("uid=%s,%s", ldap.EscapeDN(PolicySelfTestUID), c.UserBaseDN())
	test := &PolicySelfTest{Result: EnforcementUnknown, ProbeDN: dn}
	step := func(format string, args ...any) {
		test.Steps = append(test.Steps, fmt.Sprintf(format, args...))
	}

	// Never touch an entry we did not create.
	if existing, err := c.GetEntry(dn, []string{"dn"}); err == nil && existing != nil {
		return nil, ErrSelfTestEntryExists
	}

	password, err := randomPassword()
	if err != nil {
		return nil, fmt.Errorf("generate probe password: %w", err)
	}

	addReq := ldap.NewAddRequest(dn, nil)
	addReq.Attribute("objectClass", []string{"inetOrgPerson"})
	addReq.Attribute("cn", []string{"LDAP Warden policy self-test"})
	addReq.Attribute("sn", []string{"self-test"})
	addReq.Attribute("uid", []string{PolicySelfTestUID})
	addReq.Attribute("description", []string{
		"Temporary entry created by LDAP Warden to verify password-policy enforcement. Safe to delete."})
	if err := c.Add(addReq); err != nil {
		return nil, fmt.Errorf("create probe entry: %w", err)
	}
	step("created %s", dn)

	// From here the entry exists, so every exit removes it.
	defer func() {
		if err := c.Delete(dn); err != nil {
			test.Steps = append(test.Steps, fmt.Sprintf("FAILED to remove %s: %v", dn, err))
			test.CleanedUp = false
			return
		}
		test.Steps = append(test.Steps, fmt.Sprintf("removed %s", dn))
		test.CleanedUp = true
	}()

	// PasswordModify rather than a plain attribute write, so the overlay hashes
	// and quality-checks it the way it would a real password.
	if err := c.PasswordModify(dn, password); err != nil {
		test.Detail = fmt.Sprintf("could not set a password on the probe entry: %v", err)
		return test, nil
	}
	step("set a password via PasswordModify")

	// The control: an unlocked probe entry must be able to bind, otherwise the
	// test below proves nothing.
	if err := c.Bind(dn, password); err != nil {
		test.Detail = fmt.Sprintf(
			"the probe entry could not authenticate even before being locked, so locking proves nothing: %v", err)
		return test, nil
	}
	step("bind succeeded before locking (control)")

	modReq := ldap.NewModifyRequest(dn, nil)
	modReq.Replace(lockAttr, []string{permanentLockValue})
	if err := c.Modify(modReq); err != nil {
		// Writing the attribute failed outright — that happens when the overlay is
		// not loaded at all, which is a definitive "nothing will be enforced".
		test.Result = EnforcementNotEnforced
		test.Detail = fmt.Sprintf(
			"%s could not be written at all, so the ppolicy overlay is not loaded: %v", lockAttr, err)
		return test, nil
	}
	step("wrote %s = %s", lockAttr, permanentLockValue)

	if err := c.Bind(dn, password); err != nil {
		test.Result = EnforcementEnforced
		step("bind refused after locking — the directory enforces the policy")
		return test, nil
	}

	test.Result = EnforcementNotEnforced
	step("bind still succeeded after locking")
	test.Detail = "the account was locked and the directory let it authenticate anyway: " +
		"no password policy applies to new entries in this OU, so locking and account expiry will have no effect. " +
		"Set olcPPolicyDefault on the ppolicy overlay, or a pwdPolicySubentry on each entry."
	return test, nil
}

// randomPassword returns a password long enough to satisfy any plausible
// pwdMinLength and pwdCheckQuality, so the test measures enforcement rather than
// tripping over a quality rule.
func randomPassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	// Mixed case, digits and a symbol, whatever the random bytes encode to.
	return "Aa1!" + base64.RawURLEncoding.EncodeToString(buf), nil
}
