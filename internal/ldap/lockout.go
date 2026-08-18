package ldap

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
)

// LockState says what an entry's pwdAccountLockedTime means *right now*.
//
// The attribute on its own cannot say. ppolicy lifts a timestamped lock once
// pwdLockoutDuration has elapsed, but it has no timer: it re-reads the value at
// bind time, and deletes it there. So a lock posted after a run of failed binds
// sits in the entry for as long as nobody tries to authenticate — hours or
// weeks after it stopped refusing anything. Reporting "locked" from the mere
// presence of the attribute claims a lockout that no longer exists, which is
// what this type exists to stop.
type LockState string

const (
	// LockStateNone: no pwdAccountLockedTime on the entry.
	LockStateNone LockState = "none"
	// LockStateManual: the permanent-lock value written by LockUser. ppolicy
	// special-cases it and never lifts it, whatever the policy's duration says.
	LockStateManual LockState = "manual"
	// LockStateActive: a timestamped lock still within the policy's lockout
	// duration. Lockout.LiftsAt says when it stops — unless the duration is 0,
	// which means it never does.
	LockStateActive LockState = "active"
	// LockStateLifted: a timestamped lock whose duration has elapsed. The
	// account authenticates; the attribute is stale, and the directory drops it
	// at the next bind on the entry.
	LockStateLifted LockState = "lifted"
	// LockStateUnknown: a timestamped lock whose applicable policy could not be
	// read, so whether it still holds is not knowable from here. Counts as
	// locked wherever a yes/no is needed: declining to guess must never report a
	// locked account as usable.
	LockStateUnknown LockState = "unknown"
)

// Lockout is the resolved lock situation of one entry.
type Lockout struct {
	State LockState `json:"state"`
	// LockedAt is pwdAccountLockedTime verbatim, so a caller can show the same
	// instant the directory holds. Empty when the entry carries no lock.
	LockedAt string `json:"lockedAt,omitempty"`
	// LiftsAt is when the lockout duration runs out, RFC 3339 in UTC. Set for
	// LockStateActive and LockStateLifted — in the past for the latter, which is
	// precisely what makes it lifted.
	LiftsAt string `json:"liftsAt,omitempty"`
	// DurationSeconds is the applicable policy's pwdLockoutDuration, filled only
	// where it was needed — under LockStateActive and LockStateLifted. 0 there
	// means a lock never lifts on its own.
	DurationSeconds int `json:"durationSeconds"`
	// PolicyDN is the policy that governs this entry as far as the app knows: the
	// entry's own pwdPolicySubentry, or LDAP_DEFAULT_POLICY_DN standing in for the
	// server-wide olcPPolicyDefault that no ordinary bind identity can read. Set
	// whether or not the entry is locked, and empty when neither is configured —
	// which is exactly when a timestamped lock resolves to LockStateUnknown.
	PolicyDN string `json:"policyDn,omitempty"`
}

// Locked reports whether a lock is refusing binds on this entry. Only "no lock"
// and "lapsed lock" answer false; unknown answers true, because a lock that
// might hold has to be reported as one.
func (l *Lockout) Locked() bool {
	if l == nil {
		return false
	}
	return l.State != LockStateNone && l.State != LockStateLifted
}

// lockFloor is a generalizedTime below every lock a directory posts and above
// permanentLockValue, whose year is 0000.
//
// It exists so a filter can exclude the permanent lock by ordering instead of by
// equality: asserting equality against year 0000 asks every server's
// generalizedTime validator to accept a value slapd only ever writes internally.
const lockFloor = "19700101000000Z"

// lockoutResolver turns pwdAccountLockedTime values into a LockState, reading
// each password policy at most once.
//
// One instance per request. It caches, so a listing of 500 users costs one
// policy read rather than 500, and a policy edited between two requests is
// picked up by the next one — no stale duration outlives the response it was
// read for.
type lockoutResolver struct {
	client *Client
	now    time.Time
	// policies maps a policy DN to the policy, or to nil when it could not be
	// read. The nil entries matter as much as the others: they stop a listing
	// from retrying an unreadable DN once per user.
	policies map[string]*PasswordPolicy
}

func (c *Client) newLockoutResolver() *lockoutResolver {
	return &lockoutResolver{
		client:   c,
		now:      time.Now(),
		policies: make(map[string]*PasswordPolicy),
	}
}

// resolve describes the lock on an entry with this pwdAccountLockedTime and
// pwdPolicySubentry. Both may be empty; the result is never nil.
func (r *lockoutResolver) resolve(lockedAt, subentry string) *Lockout {
	policyDN := subentry
	if policyDN == "" {
		policyDN = r.client.config.DefaultPolicyDN
	}

	// Only a timestamped lock needs a duration, so only that case pays for the
	// policy read: no lock and a permanent lock are decided by the value alone.
	var policy *PasswordPolicy
	if lockedAt != "" && lockedAt != permanentLockValue {
		policy = r.policy(policyDN)
	}

	return lockoutFor(lockedAt, policyDN, policy, r.now)
}

// lockoutFor is the state machine, with the applicable policy already read — nil
// when there is none to read or reading it failed, which is the same answer
// either way: the duration is not known.
func lockoutFor(lockedAt, policyDN string, policy *PasswordPolicy, now time.Time) *Lockout {
	if lockedAt == "" {
		return &Lockout{State: LockStateNone, PolicyDN: policyDN}
	}

	out := &Lockout{LockedAt: lockedAt, PolicyDN: policyDN}
	if lockedAt == permanentLockValue {
		out.State = LockStateManual
		return out
	}

	since, ok := parseGeneralizedTime(lockedAt)
	if !ok {
		// slapd cannot read it either, and treats what it cannot parse as a lock
		// rather than as an absence. Report the same instead of inventing an
		// expiry for it.
		out.State = LockStateUnknown
		return out
	}

	if policy == nil {
		out.State = LockStateUnknown
		return out
	}

	out.DurationSeconds = policy.PwdLockoutDuration
	if policy.PwdLockoutDuration <= 0 {
		// pwdLockoutDuration 0 is the ppolicy default, and it means "until an
		// administrator unlocks it" — the same outcome as a manual lock, reached
		// by not configuring anything.
		out.State = LockStateActive
		return out
	}

	liftsAt := since.Add(time.Duration(policy.PwdLockoutDuration) * time.Second)
	out.LiftsAt = liftsAt.UTC().Format(time.RFC3339)
	// The same comparison ppolicy makes at bind time: locked while now is before
	// lockedAt + duration.
	if now.Before(liftsAt) {
		out.State = LockStateActive
	} else {
		out.State = LockStateLifted
	}
	return out
}

// policy reads a password policy, once per resolver.
func (r *lockoutResolver) policy(dn string) *PasswordPolicy {
	if dn == "" {
		return nil
	}
	if policy, cached := r.policies[dn]; cached {
		return policy
	}

	policy, err := r.client.GetPasswordPolicy(dn)
	if err != nil {
		policy = nil
	}
	r.policies[dn] = policy
	return policy
}

// lockedNowFilter is the counting counterpart of lockoutResolver: an LDAP filter
// matching entries whose pwdAccountLockedTime still refuses binds, for the
// dashboard figures — which count rather than fetch, so they cannot resolve a
// lock per entry.
//
// It degrades to "(pwdAccountLockedTime=*)" — every lock, lapsed ones included,
// which is what this used to count unconditionally — whenever no duration is
// known. That is the conservative direction: a lapsed lock counted as locked
// overstates the figure, while the opposite would hide a locked account.
func (c *Client) lockedNowFilter() string {
	present := fmt.Sprintf("(%s=*)", lockAttr)
	lapsed := c.lapsedLockFilter(time.Now())
	if lapsed == "" {
		return present
	}
	return fmt.Sprintf("(&%s(!%s))", present, lapsed)
}

// lapsedLockFilter matches entries carrying a timestamped lock that its policy's
// pwdLockoutDuration has already lifted, or "" when no policy states a duration.
//
// A lock lapsed when lockedAt + duration <= now, i.e. lockedAt <= now - duration:
// one cutoff per policy, matched against the entries that policy governs. Entries
// whose pwdPolicySubentry names a policy this cannot read are matched by no
// branch, and so stay counted as locked.
func (c *Client) lapsedLockFilter(now time.Time) string {
	policies, err := c.ListPasswordPolicies()
	if err != nil {
		return ""
	}
	return lapsedLockFilterFor(policies, c.config.DefaultPolicyDN, now)
}

func lapsedLockFilterFor(policies []PasswordPolicy, defaultPolicyDN string, now time.Time) string {
	var branches strings.Builder
	for _, policy := range policies {
		if policy.PwdLockoutDuration <= 0 {
			continue
		}
		cutoff := GeneralizedTime(now.Add(-time.Duration(policy.PwdLockoutDuration) * time.Second))

		fmt.Fprintf(&branches, "(&(pwdPolicySubentry=%s)(%s<=%s))",
			ldap.EscapeFilter(policy.DN), lockAttr, cutoff)

		// Entries with no policy of their own follow the directory's default, so
		// its cutoff applies to exactly those. Unset LDAP_DEFAULT_POLICY_DN and
		// this branch is absent, leaving every such lock counted.
		if sameDN(policy.DN, defaultPolicyDN) {
			fmt.Fprintf(&branches, "(&(!(pwdPolicySubentry=*))(%s<=%s))", lockAttr, cutoff)
		}
	}

	if branches.Len() == 0 {
		return ""
	}
	// permanentLockValue sits below every cutoff, being year 0000, so it would
	// match each branch. The floor rules it out without naming it.
	return fmt.Sprintf("(&(%s>=%s)(|%s))", lockAttr, lockFloor, branches.String())
}

// sameDN reports whether two DNs designate the same entry, by LDAP
// distinguishedNameMatch semantics rather than string equality: a configured
// "CN=default, ou=policies,dc=example,dc=org" is the policy the directory
// returns as "cn=default,ou=policies,dc=example,dc=org".
func sameDN(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	parsedA, err := ldap.ParseDN(a)
	if err != nil {
		return false
	}
	parsedB, err := ldap.ParseDN(b)
	if err != nil {
		return false
	}
	return parsedA.EqualFold(parsedB)
}
