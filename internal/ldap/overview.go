package ldap

import (
	"fmt"
	"sort"
	"time"
)

// ExpiringAccount is a user whose account expires within the overview window.
type ExpiringAccount struct {
	DN          string    `json:"dn"`
	UID         string    `json:"uid"`
	DisplayName string    `json:"displayName,omitempty"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// UserOverview summarises the user population.
type UserOverview struct {
	Total    int               `json:"total"`
	Active   int               `json:"active"`
	Locked   int               `json:"locked"`
	Expired  int               `json:"expired"`
	Expiring []ExpiringAccount `json:"expiring"`
}

// Overview is the directory at a glance, as shown on the dashboard.
//
// Counts are computed here rather than in the browser: the alternative is
// shipping every entry to the client just to length-check it, which is exactly
// what the list endpoints stopped doing.
type Overview struct {
	Users     UserOverview `json:"users"`
	Groups    int          `json:"groups"`
	SudoRoles int          `json:"sudoRoles"`
	Policies  int          `json:"policies"`
	// Truncated reports that the *server* stopped a count early, so the figures
	// are lower bounds. It used to mean the app had stopped at its own
	// SearchSizeLimit, which made this warning permanent on any directory of more
	// than 500 entries and gave the operator nothing to act on. Now it only fires
	// when olcSizeLimit (or the equivalent) bounds the bind identity, which is a
	// real thing to go and change.
	Truncated bool `json:"truncated"`
}

// overviewExpiringDays is how far ahead the dashboard looks for expirations. It
// matches the first reminder the expiration scheduler sends (three weeks),
// rounded up to a month, so the dashboard warns about the same accounts the
// admins are already being emailed about.
const overviewExpiringDays = 30

// userFilter selects the entries the dashboard counts as accounts.
const userFilter = "(objectClass=inetOrgPerson)"

// overviewExpiringLimit caps the expiring list; it is a prompt to act, not a
// report. The full set is one click away on the users list.
const overviewExpiringLimit = 5

// GetOverview collects the dashboard figures. A module whose schema is absent
// from the directory (sudo, ppolicy) yields a zero count rather than an error:
// the dashboard should still render on a directory that does not use them.
//
// Every figure is a count, not a length: the directory is asked "how many match
// this filter" with no attributes requested, instead of being made to hand over
// every user entry so they can be tallied here. On a 706-entry directory that
// moved 55 kB where the old capped-and-wrong version moved 82 kB; the saving
// narrows as the directory grows, since only the old version was bounded.
//
// The filters on pwdAccountLockedTime and pwdEndTime are unindexed in a default
// slapd configuration, so these are candidate scans. Adding an index for both is
// the fix if the dashboard gets slow on a large directory.
func (c *Client) GetOverview() (*Overview, error) {
	now := time.Now()
	nowFilter := GeneralizedTime(now)
	horizon := now.AddDate(0, 0, overviewExpiringDays)
	horizonFilter := GeneralizedTime(horizon)

	overview := &Overview{}

	// required counts a figure that the dashboard cannot be honest without. A
	// failure is returned, not swallowed: Active is derived by subtraction, so a
	// silent zero here would render "Nothing needs attention right now" over a
	// directory full of locked accounts. Better a failed dashboard than a
	// reassuring one.
	required := func(filter string) (int, error) {
		n, serverLimited, err := c.CountEntries(c.UserBaseDN(), filter)
		if err != nil {
			return 0, err
		}
		overview.Truncated = overview.Truncated || serverLimited
		return n, nil
	}

	// optional counts a collection whose schema a directory may legitimately not
	// have (sudo, ppolicy). Zero is the right answer there, and the only one the
	// caller can act on.
	optional := func(baseDN, filter string) int {
		n, serverLimited, err := c.CountEntries(baseDN, filter)
		if err != nil {
			return 0
		}
		overview.Truncated = overview.Truncated || serverLimited
		return n
	}

	total, err := required(userFilter)
	if err != nil {
		return nil, fmt.Errorf("count users: %w", err)
	}
	overview.Users.Total = total

	// Locks that are still refusing binds, not every entry carrying the attribute:
	// ppolicy leaves a lapsed lockout in place until the entry's next bind, and
	// counting those makes the figure disagree with the users list, which resolves
	// each row against its policy.
	lockedNow := c.lockedNowFilter()
	lockedFilter := fmt.Sprintf("(&%s%s)", userFilter, lockedNow)
	if overview.Users.Locked, err = required(lockedFilter); err != nil {
		return nil, fmt.Errorf("count locked accounts: %w", err)
	}

	// Only ask about pwdEndTime where it exists. A filter naming an unknown
	// attribute evaluates to Undefined and matches nothing, which happens to be
	// the right answer here — but relying on that is relying on the server being
	// lenient about a filter it should arguably reject.
	inactiveFilter := lockedFilter
	if c.Capabilities().ScheduledExpiry {
		if overview.Users.Expired, err = required(
			fmt.Sprintf("(&%s(%s<=%s))", userFilter, expiryAttr, nowFilter)); err != nil {
			return nil, fmt.Errorf("count expired accounts: %w", err)
		}
		inactiveFilter = fmt.Sprintf("(&%s(|%s(%s<=%s)))",
			userFilter, lockedNow, expiryAttr, nowFilter)
	}

	// Active is the complement of the union, not total minus the two counts: an
	// account can be locked *and* expired, and subtracting both would charge it
	// twice.
	//
	// Both being zero makes the union zero, so the extra scan is skipped — worth
	// it because these filters are unindexed in a default configuration and this
	// is the state a healthy directory is in.
	inactive := 0
	switch {
	case overview.Users.Locked == 0 && overview.Users.Expired == 0:
	case inactiveFilter == lockedFilter:
		// No expiry support: the union is exactly the locked set, already counted.
		inactive = overview.Users.Locked
	default:
		if inactive, err = required(inactiveFilter); err != nil {
			return nil, fmt.Errorf("count inactive accounts: %w", err)
		}
	}
	overview.Users.Active = overview.Users.Total - inactive
	if overview.Users.Active < 0 {
		overview.Users.Active = 0
	}

	overview.Users.Expiring = c.expiringAccounts(nowFilter, horizonFilter, horizon)

	overview.Groups = optional(c.GroupBaseDN(), "(objectClass=posixGroup)")
	overview.SudoRoles = optional(c.SudoBaseDN(), "(objectClass=sudoRole)")
	overview.Policies = optional(c.PpolicyBaseDN(), "(objectClass=pwdPolicy)")

	return overview, nil
}

// expiringAccounts lists the accounts whose expiry falls inside the dashboard
// window, soonest first.
//
// The filter bounds the result to the window, so this fetches a handful of
// entries rather than scanning the whole population to pick them out. An empty
// list on a directory without pwdEndTime is the correct answer, not a failure.
func (c *Client) expiringAccounts(nowFilter, horizonFilter string, horizon time.Time) []ExpiringAccount {
	expiring := make([]ExpiringAccount, 0)
	if !c.Capabilities().ScheduledExpiry {
		return expiring
	}

	filter := fmt.Sprintf("(&%s(%s>=%s)(%s<=%s))",
		userFilter, expiryAttr, nowFilter, expiryAttr, horizonFilter)
	// Bounded, unlike the counts: this is the one search here that transfers
	// attribute values, and a cohort provisioned with the same end-of-quarter
	// expiry puts every one of its members inside the window. LDAP has no
	// server-side sort, so the soonest five have to be picked here — from a
	// bounded sample when the window is enormous, which is the same compromise
	// overviewExpiringLimit already makes. The full set is one click away on the
	// users list.
	entries, _, err := c.SearchLimited(c.UserBaseDN(), filter,
		[]string{"uid", "cn", "displayName", expiryAttr})
	if err != nil {
		return expiring
	}

	for _, entry := range entries {
		item := entryToUserListItem(entry)
		expiresAt, ok := item.expiresAt()
		if !ok {
			// The server matched this entry against the window, so it *is*
			// expiring; only this parse disagrees. Dropping it would hide an
			// account the directory is about to disable, and would contradict the
			// counts, which come from the server's own view. Show it at the far end
			// of the window rather than not at all.
			expiresAt = horizon
		}
		expiring = append(expiring, ExpiringAccount{
			DN:          item.DN,
			UID:         item.UID,
			DisplayName: displayNameOf(item),
			ExpiresAt:   expiresAt,
		})
	}

	// Soonest first: the dashboard shows what needs attention next.
	sort.Slice(expiring, func(i, j int) bool { return expiring[i].ExpiresAt.Before(expiring[j].ExpiresAt) })
	if len(expiring) > overviewExpiringLimit {
		expiring = expiring[:overviewExpiringLimit]
	}
	return expiring
}

// generalizedTimeLayouts are the RFC 4517 forms a directory may hand back. The
// app writes the first one, but pwdEndTime is a plain attribute that other tooling
// may have set with an explicit offset or a fraction of a second — and the server
// compares all of them correctly in a filter, so a value it counts as expired must
// not be one this cannot read.
var generalizedTimeLayouts = []string{
	generalizedTimeLayout, // 20260810120000Z
	"20060102150405-0700", // 20260810120000+0100
	"20060102150405.000Z",
	"20060102150405.000-0700",
	"200601021504Z", // minute granularity
	"2006010215Z",   // hour granularity
}

// parseGeneralizedTime reads a generalizedTime in any of the forms above,
// reporting false for a value none of them accept.
func parseGeneralizedTime(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range generalizedTimeLayouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// expiresAt returns the instant the directory will start refusing this account,
// from pwdEndTime. A value none of the layouts accept is reported as "no expiry"
// rather than guessed at — slapd itself treats an unparseable value as locked, and
// that shows up as the lock state instead.
func (u UserListItem) expiresAt() (time.Time, bool) {
	return parseGeneralizedTime(u.PwdEndTime)
}
