package ldap

import (
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
	// Truncated reports that a count hit the configured search size limit and is
	// therefore a lower bound — the dashboard says so rather than showing a
	// plausible wrong number.
	Truncated bool `json:"truncated"`
}

// overviewExpiringDays is how far ahead the dashboard looks for expirations. It
// matches the first reminder the expiration scheduler sends (three weeks),
// rounded up to a month, so the dashboard warns about the same accounts the
// admins are already being emailed about.
const overviewExpiringDays = 30

// overviewExpiringLimit caps the expiring list; it is a prompt to act, not a
// report. The full set is one click away on the users list.
const overviewExpiringLimit = 5

// GetOverview collects the dashboard figures. A module whose schema is absent
// from the directory (sudo, ppolicy) yields a zero count rather than an error:
// the dashboard should still render on a directory that does not use them.
func (c *Client) GetOverview() (*Overview, error) {
	entries, truncated, err := c.SearchLimited(c.UserBaseDN(), "(objectClass=inetOrgPerson)", listUserAttributes)
	if err != nil {
		return nil, err
	}

	overview := &Overview{Truncated: truncated}
	now := time.Now()
	horizon := now.AddDate(0, 0, overviewExpiringDays)
	expiring := make([]ExpiringAccount, 0)

	for _, entry := range entries {
		item := entryToUserListItem(entry)
		overview.Users.Total++

		switch {
		case item.AccountLocked:
			overview.Users.Locked++
		case item.expired():
			overview.Users.Expired++
		default:
			overview.Users.Active++
		}

		if expiresAt, ok := item.expiresAt(); ok && expiresAt.After(now) && expiresAt.Before(horizon) {
			expiring = append(expiring, ExpiringAccount{
				DN:          item.DN,
				UID:         item.UID,
				DisplayName: displayNameOf(item),
				ExpiresAt:   expiresAt,
			})
		}
	}

	// Soonest first: the dashboard shows what needs attention next.
	sort.Slice(expiring, func(i, j int) bool { return expiring[i].ExpiresAt.Before(expiring[j].ExpiresAt) })
	if len(expiring) > overviewExpiringLimit {
		expiring = expiring[:overviewExpiringLimit]
	}
	overview.Users.Expiring = expiring

	if groups, groupsTruncated, err := c.SearchLimited(c.GroupBaseDN(), "(objectClass=posixGroup)", []string{"dn"}); err == nil {
		overview.Groups = len(groups)
		overview.Truncated = overview.Truncated || groupsTruncated
	}
	if roles, _, err := c.SearchLimited(c.SudoBaseDN(), "(objectClass=sudoRole)", []string{"dn"}); err == nil {
		overview.SudoRoles = len(roles)
	}
	if policies, _, err := c.SearchLimited(c.PpolicyBaseDN(), "(objectClass=pwdPolicy)", []string{"dn"}); err == nil {
		overview.Policies = len(policies)
	}

	return overview, nil
}

// expiresAt returns the instant the directory will start refusing this account,
// from pwdEndTime. A value the server cannot parse is reported as "no expiry"
// rather than guessed at — slapd itself treats an unparseable value as locked, and
// that shows up as the lock state instead.
func (u UserListItem) expiresAt() (time.Time, bool) {
	if u.PwdEndTime == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("20060102150405Z", u.PwdEndTime)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
