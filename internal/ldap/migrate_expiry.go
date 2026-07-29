package ldap

import (
	"fmt"
	"time"

	"github.com/go-ldap/ldap/v3"
)

// ExpiryMigrationOptions controls which of the legacy expiry encodings are moved
// to pwdEndTime.
type ExpiryMigrationOptions struct {
	// Apply writes the changes. Left false, the migration only reports what it
	// would do — the default, because the risky cases below are judgement calls.
	Apply bool

	// IncludePastLocks also migrates pwdAccountLockedTime values in the past.
	//
	// Off by default, and deliberately: such a value has no effect today (ppolicy
	// lifts a timestamped lock once pwdLockoutDuration elapses), so those accounts
	// currently work. Moving them to pwdEndTime would newly refuse them — correct
	// if the date was meant as an expiry, a mass lockout if it was a real lock.
	IncludePastLocks bool

	// IncludeShadowExpire also migrates shadowExpire to pwdEndTime.
	//
	// Off by default for the same reason: slapd ignores shadowExpire, so an
	// account with a lapsed one still binds today. It also stops PAM/SSSD on the
	// hosts from enforcing it, since the attribute is left untouched but the
	// authority moves.
	IncludeShadowExpire bool
}

// ExpiryMigrationEntry is one account the migration looked at.
type ExpiryMigrationEntry struct {
	DN  string
	UID string
	// From names the attribute the value was read from, and Value renders it.
	From  string
	Value string
	// To is the pwdEndTime value that was (or would be) written.
	To string
	// Action is "migrate", "skip" or "error", and Reason explains anything but
	// the first.
	Action string
	Reason string
}

// ExpiryMigrationReport summarises a run.
type ExpiryMigrationReport struct {
	Applied  bool
	Migrated int
	Skipped  int
	Failed   int
	Entries  []ExpiryMigrationEntry
}

// MigrateAccountExpiry moves account expirations from the attributes earlier
// versions used onto pwdEndTime.
//
// Two encodings existed, neither of which the directory enforces as an expiry:
//
//   - pwdAccountLockedTime, which OpenLDAP reads as "a lockout started at this
//     instant". A future value therefore disabled the account immediately, which
//     is the bug this migration repairs; a past one silently stopped applying.
//   - shadowExpire, which slapd ignores entirely (only PAM/SSSD read it).
//
// The default run migrates exactly the first case — future pwdAccountLockedTime
// values, i.e. accounts that were scheduled to expire and are broken right now —
// and reports the rest for a human to decide on.
func (c *Client) MigrateAccountExpiry(opts ExpiryMigrationOptions) (*ExpiryMigrationReport, error) {
	if !c.Capabilities().ScheduledExpiry {
		return nil, ErrScheduledExpiryUnsupported
	}

	entries, _, err := c.SearchLimited(c.UserBaseDN(), "(objectClass=inetOrgPerson)",
		[]string{"uid", lockAttr, expiryAttr, "shadowExpire"})
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	report := &ExpiryMigrationReport{Applied: opts.Apply}
	now := time.Now()

	for _, entry := range entries {
		item := ExpiryMigrationEntry{DN: entry.DN, UID: entry.GetAttributeValue("uid")}

		// An entry that already carries an expiry is left alone: it was either
		// migrated or set by a current version, and overwriting it would lose the
		// operator's intent.
		if existing := entry.GetAttributeValue(expiryAttr); existing != "" {
			continue
		}

		lock := entry.GetAttributeValue(lockAttr)
		shadow := entry.GetAttributeValue("shadowExpire")

		switch {
		case lock != "" && lock != permanentLockValue:
			parsed, perr := time.Parse("20060102150405Z", lock)
			if perr != nil {
				// Includes any sentinel we do not recognise. Not an expiry, and not
				// ours to reinterpret.
				continue
			}
			item.From, item.Value, item.To = lockAttr, lock, lock
			if parsed.After(now) {
				item.Action = "migrate"
			} else if opts.IncludePastLocks {
				item.Action = "migrate"
			} else {
				item.Action = "skip"
				item.Reason = "lock timestamp is in the past: it may be a real lock, and migrating it would refuse an account that currently works (--include-past-locks)"
			}

		case shadow != "":
			days, serr := parseShadowExpire(shadow)
			if serr != nil || days <= 0 || days >= ShadowExpireNever {
				continue
			}
			expiresAt := time.Unix(int64(days)*86400, 0).UTC()
			item.From, item.Value = "shadowExpire", shadow
			item.To = expiresAt.Format("20060102150405Z")
			if opts.IncludeShadowExpire {
				item.Action = "migrate"
			} else {
				item.Action = "skip"
				item.Reason = "shadowExpire is enforced by PAM/SSSD on hosts, not by slapd; migrating changes who enforces it (--include-shadow-expire)"
			}

		default:
			continue
		}

		if item.Action == "skip" {
			report.Skipped++
			report.Entries = append(report.Entries, item)
			continue
		}

		if opts.Apply {
			if err := c.writeMigratedExpiry(entry.DN, item.From, item.To); err != nil {
				item.Action, item.Reason = "error", err.Error()
				report.Failed++
				report.Entries = append(report.Entries, item)
				continue
			}
		}
		report.Migrated++
		report.Entries = append(report.Entries, item)
	}

	return report, nil
}

// writeMigratedExpiry sets pwdEndTime and, when the value came from the lock
// attribute, removes it — leaving it would keep the account disabled.
// shadowExpire is left in place: the hosts may still be reading it.
func (c *Client) writeMigratedExpiry(dn, from, value string) error {
	modReq := ldap.NewModifyRequest(dn, nil)
	modReq.Add(expiryAttr, []string{value})
	if from == lockAttr {
		modReq.Delete(lockAttr, []string{})
	}
	if err := c.Modify(modReq); err != nil {
		return fmt.Errorf("migrate %s: %w", dn, err)
	}
	return nil
}

func parseShadowExpire(value string) (int, error) {
	var days int
	_, err := fmt.Sscanf(value, "%d", &days)
	return days, err
}

// SetRawAttribute replaces a single attribute value. It exists for the migration
// tests, which have to plant the legacy encoding this code is meant to clean up —
// there is deliberately no product path that writes an expiry to the lock
// attribute any more.
func (c *Client) SetRawAttribute(dn, attribute, value string) error {
	modReq := ldap.NewModifyRequest(dn, nil)
	modReq.Replace(attribute, []string{value})
	return c.Modify(modReq)
}
