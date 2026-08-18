package ldap

import (
	"strings"
	"testing"
	"time"
)

const testPolicyDN = "cn=default,ou=policies,dc=example,dc=org"

func TestLockoutFor(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	lockedAt := func(d time.Duration) string { return GeneralizedTime(now.Add(-d)) }

	timed := &PasswordPolicy{DN: testPolicyDN, PwdLockout: true, PwdLockoutDuration: 900}
	permanent := &PasswordPolicy{DN: testPolicyDN, PwdLockout: true}

	tests := []struct {
		name        string
		lockedAt    string
		policy      *PasswordPolicy
		wantState   LockState
		wantLocked  bool
		wantLiftsAt string
	}{
		{
			name:      "no lock",
			policy:    timed,
			wantState: LockStateNone,
		},
		{
			// The value LockUser writes. It must survive any duration: an
			// administrator's lock that quietly lifted itself after 15 minutes is the
			// bug this whole state machine exists to prevent.
			name:       "permanent lock value is never lifted",
			lockedAt:   permanentLockValue,
			policy:     timed,
			wantState:  LockStateManual,
			wantLocked: true,
		},
		{
			name:        "timestamped lock inside the duration",
			lockedAt:    lockedAt(5 * time.Minute),
			policy:      timed,
			wantState:   LockStateActive,
			wantLocked:  true,
			wantLiftsAt: "2026-08-18T12:10:00Z",
		},
		{
			// The case that had the UI reporting a lock for a day: ppolicy stopped
			// refusing binds at 10:15, and left the attribute behind after that.
			name:        "timestamped lock past the duration",
			lockedAt:    lockedAt(2 * time.Hour),
			policy:      timed,
			wantState:   LockStateLifted,
			wantLocked:  false,
			wantLiftsAt: "2026-08-18T10:15:00Z",
		},
		{
			// Exactly at the boundary the directory lets the bind through: it locks
			// while now < lockedAt + duration.
			name:        "timestamped lock at the boundary",
			lockedAt:    lockedAt(900 * time.Second),
			policy:      timed,
			wantState:   LockStateLifted,
			wantLocked:  false,
			wantLiftsAt: "2026-08-18T12:00:00Z",
		},
		{
			name:       "pwdLockoutDuration 0 never lifts",
			lockedAt:   lockedAt(10 * 24 * time.Hour),
			policy:     permanent,
			wantState:  LockStateActive,
			wantLocked: true,
		},
		{
			name:       "unreadable policy leaves the answer unknown",
			lockedAt:   lockedAt(2 * time.Hour),
			policy:     nil,
			wantState:  LockStateUnknown,
			wantLocked: true,
		},
		{
			// slapd treats a value it cannot parse as a lock, so neither may this
			// report the account as usable.
			name:       "unparseable lock value counts as locked",
			lockedAt:   "not-a-timestamp",
			policy:     timed,
			wantState:  LockStateUnknown,
			wantLocked: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := lockoutFor(tc.lockedAt, testPolicyDN, tc.policy, now)

			if got.State != tc.wantState {
				t.Errorf("state = %q, want %q", got.State, tc.wantState)
			}
			if got.Locked() != tc.wantLocked {
				t.Errorf("Locked() = %v, want %v", got.Locked(), tc.wantLocked)
			}
			if got.LiftsAt != tc.wantLiftsAt {
				t.Errorf("liftsAt = %q, want %q", got.LiftsAt, tc.wantLiftsAt)
			}
			if got.PolicyDN != testPolicyDN {
				t.Errorf("policyDN = %q, want %q", got.PolicyDN, testPolicyDN)
			}
		})
	}
}

// TestLockoutLockedNil guards the fallback the API relies on: a caller that
// never resolved a lockout must not read as unlocked by accident.
func TestLockoutLockedNil(t *testing.T) {
	var absent *Lockout
	if absent.Locked() {
		t.Error("a nil Lockout reports a lock")
	}
}

func TestLapsedLockFilterFor(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	strict := PasswordPolicy{DN: "cn=strict,ou=policies,dc=example,dc=org", PwdLockoutDuration: 3600}
	def := PasswordPolicy{DN: testPolicyDN, PwdLockoutDuration: 900}
	forever := PasswordPolicy{DN: "cn=forever,ou=policies,dc=example,dc=org"}

	t.Run("no policy states a duration", func(t *testing.T) {
		// Nothing can be ruled out, so nothing is: every lock keeps counting.
		if got := lapsedLockFilterFor([]PasswordPolicy{forever}, testPolicyDN, now); got != "" {
			t.Errorf("filter = %q, want empty", got)
		}
	})

	t.Run("one cutoff per policy plus the configured default", func(t *testing.T) {
		got := lapsedLockFilterFor([]PasswordPolicy{strict, def, forever}, testPolicyDN, now)

		for _, want := range []string{
			// Excludes the permanent lock, whose year 0000 sorts below every cutoff.
			"(pwdAccountLockedTime>=19700101000000Z)",
			"(&(pwdPolicySubentry=cn=strict,ou=policies,dc=example,dc=org)(pwdAccountLockedTime<=20260818110000Z))",
			"(&(pwdPolicySubentry=cn=default,ou=policies,dc=example,dc=org)(pwdAccountLockedTime<=20260818114500Z))",
			// Entries with no policy of their own follow the default's cutoff.
			"(&(!(pwdPolicySubentry=*))(pwdAccountLockedTime<=20260818114500Z))",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("filter %q\n  missing %q", got, want)
			}
		}
		if strings.Contains(got, "cn=forever") {
			t.Errorf("filter %q names a policy with no lockout duration", got)
		}
	})

	t.Run("no configured default leaves those entries counted", func(t *testing.T) {
		got := lapsedLockFilterFor([]PasswordPolicy{def}, "", now)
		if strings.Contains(got, "(!(pwdPolicySubentry=*))") {
			t.Errorf("filter %q assumes a default policy that was never declared", got)
		}
	})

	t.Run("the default is matched as a DN, not as a string", func(t *testing.T) {
		got := lapsedLockFilterFor([]PasswordPolicy{def}, "CN=default, ou=policies,DC=example,DC=org", now)
		if !strings.Contains(got, "(!(pwdPolicySubentry=*))") {
			t.Errorf("filter %q missed the default policy over DN spelling", got)
		}
	})
}
