package scheduler

import (
	"testing"
	"time"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

func TestAttemptSpread(t *testing.T) {
	base := time.Date(2026, 8, 17, 17, 33, 42, 0, time.UTC)

	tests := []struct {
		name  string
		times []time.Time
		want  string
		ok    bool
	}{
		{name: "no attempts"},
		{name: "one attempt", times: []time.Time{base}},
		{
			// The shape of a person mistyping a password.
			name:  "a minute apart",
			times: []time.Time{base, base.Add(53 * time.Second)},
			want:  "53s",
			ok:    true,
		},
		{
			// The shape of a script, and the reason the spread is reported at all:
			// the count is always pwdMaxFailure, so it distinguishes nothing.
			name:  "sub-second",
			times: []time.Time{base, base.Add(120 * time.Millisecond)},
			want:  "0s",
			ok:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := attemptSpread(tc.times)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && got.String() != tc.want {
				t.Errorf("spread = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestLockoutEvent(t *testing.T) {
	lockedAt := time.Date(2026, 8, 17, 17, 34, 35, 0, time.UTC)
	failures := []time.Time{
		lockedAt.Add(-53 * time.Second),
		lockedAt.Add(-41 * time.Second),
		lockedAt.Add(-38 * time.Second),
		lockedAt.Add(-33 * time.Second),
		lockedAt,
	}

	t.Run("a lockout still in force", func(t *testing.T) {
		event := lockoutEvent(ldap.LockedAccount{
			UID:         "rbesson",
			DisplayName: "Romain Besson",
			LockedAt:    lockedAt,
			MaxFailure:  5,
			Lockout: &ldap.Lockout{
				State:           ldap.LockStateActive,
				LockedAt:        "20260817173435Z",
				LiftsAt:         "2026-08-17T17:49:35Z",
				DurationSeconds: 900,
			},
			FailureTimes: failures,
		})

		if event.LockedAt != "2026-08-17 17:34:35 UTC" {
			t.Errorf("lockedAt = %q", event.LockedAt)
		}
		if event.LiftsAt != "2026-08-17 17:49:35 UTC" {
			t.Errorf("liftsAt = %q", event.LiftsAt)
		}
		if event.Lifted {
			t.Error("a lockout in force is reported as lifted")
		}
		if len(event.Attempts) != 5 {
			t.Errorf("attempts = %d, want 5", len(event.Attempts))
		}
		if event.Spread != "53s" {
			t.Errorf("spread = %q, want 53s", event.Spread)
		}
		// pwdMaxFailure attempts recorded: nothing aged out, so the list is complete.
		if event.Partial {
			t.Error("a complete attempt list is flagged as partial")
		}
	})

	t.Run("a trimmed attempt list is a lower bound", func(t *testing.T) {
		// ppolicy prunes pwdFailureTime to pwdFailureCountInterval, so fewer values
		// than it took to lock means older attempts are gone — the email must not
		// present the count as what happened.
		event := lockoutEvent(ldap.LockedAccount{
			UID:          "rbesson",
			LockedAt:     lockedAt,
			MaxFailure:   5,
			Lockout:      &ldap.Lockout{State: ldap.LockStateActive, LockedAt: "20260817173435Z"},
			FailureTimes: failures[:2],
		})

		if !event.Partial {
			t.Error("a trimmed attempt list is not flagged as partial")
		}
	})

	t.Run("an unknown policy makes no claim about completeness", func(t *testing.T) {
		event := lockoutEvent(ldap.LockedAccount{
			UID:          "rbesson",
			LockedAt:     lockedAt,
			Lockout:      &ldap.Lockout{State: ldap.LockStateUnknown, LockedAt: "20260817173435Z"},
			FailureTimes: failures[:2],
		})

		if event.Partial {
			t.Error("completeness claimed without knowing pwdMaxFailure")
		}
		// No policy, so no lift instant: the template says an administrator has to act.
		if event.LiftsAt != "" {
			t.Errorf("liftsAt = %q, want empty", event.LiftsAt)
		}
	})

	t.Run("a lapsed lockout says so", func(t *testing.T) {
		event := lockoutEvent(ldap.LockedAccount{
			UID:      "rbesson",
			LockedAt: lockedAt,
			Lockout: &ldap.Lockout{
				State:    ldap.LockStateLifted,
				LockedAt: "20260817173435Z",
				LiftsAt:  "2026-08-17T17:49:35Z",
			},
		})

		if !event.Lifted {
			t.Error("a lapsed lockout is not reported as lifted")
		}
	})
}

// TestLockoutInstant guards the event identity. A lock whose timestamp cannot be
// read has no identity, and inventing one would either re-notify it on every scan
// or suppress it forever.
func TestLockoutInstant(t *testing.T) {
	if _, ok := lockoutInstant(ldap.LockedAccount{Lockout: &ldap.Lockout{LockedAt: "nonsense"}}); ok {
		t.Error("an unparseable lock timestamp yielded an instant")
	}
	if _, ok := lockoutInstant(ldap.LockedAccount{}); ok {
		t.Error("an account with no lockout yielded an instant")
	}

	lockedAt := time.Date(2026, 8, 17, 17, 34, 35, 0, time.UTC)
	got, ok := lockoutInstant(ldap.LockedAccount{LockedAt: lockedAt, Lockout: &ldap.Lockout{}})
	if !ok || !got.Equal(lockedAt) {
		t.Errorf("instant = %v (ok=%v), want %v", got, ok, lockedAt)
	}
}
