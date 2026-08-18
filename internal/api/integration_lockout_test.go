//go:build integration

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	ldaplib "github.com/go-ldap/ldap/v3"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// writeLockedTime sets pwdAccountLockedTime on an entry, the way ppolicy does
// after too many failed binds. Written directly: the API only ever writes the
// permanent-lock value, and what is under test here is how a timestamped lock is
// read back.
func writeLockedTime(t *testing.T, env *testEnv, dn, value string) {
	t.Helper()
	req := ldaplib.NewModifyRequest(dn, nil)
	req.Replace("pwdAccountLockedTime", []string{value})
	if err := env.LDAP.Modify(req); err != nil {
		t.Fatalf("write pwdAccountLockedTime=%s: %v", value, err)
	}
}

// createLockoutPolicy adds a password policy with the given lockout duration,
// and removes it when the test ends.
func createLockoutPolicy(t *testing.T, env *testEnv, durationSeconds int) *ldap.PasswordPolicy {
	t.Helper()
	cn := "testpolicy-" + uniqueSuffix(t)
	policy, err := env.LDAP.CreatePasswordPolicy(ldap.CreatePasswordPolicyRequest{
		CN:                 cn,
		Description:        "lockout resolution test",
		PwdAttribute:       "userPassword",
		PwdLockout:         true,
		PwdMaxFailure:      5,
		PwdLockoutDuration: durationSeconds,
	})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}
	t.Cleanup(func() { _ = env.LDAP.DeletePasswordPolicy(policy.DN) })
	return policy
}

// applyPolicy points an entry's pwdPolicySubentry at a policy, which is what
// makes its lockout duration the one that governs the entry — no reliance on
// olcPPolicyDefault, which the test cannot read either.
func applyPolicy(t *testing.T, env *testEnv, token, dn, policyDN string) {
	t.Helper()
	resp, body := doJSON(t, env, http.MethodPut, userPath(dn),
		map[string]any{"pwdPolicySubentry": policyDN}, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("apply policy: status=%d body=%s", resp.StatusCode, body)
	}
}

// TestIntegration_LockoutState covers the three answers a pwdAccountLockedTime
// value can carry, which the attribute on its own cannot distinguish.
//
// ppolicy has no timer: it re-reads the value at bind time and deletes it there.
// So a lockout past its pwdLockoutDuration stays in the entry until someone next
// tries to log in, and reading "locked" from the attribute's presence — which is
// what this used to do — reports a lockout that stopped holding hours ago.
func TestIntegration_LockoutState(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token
	user := createTestUser(t, env, token)
	policy := createLockoutPolicy(t, env, 900)
	applyPolicy(t, env, token, user.DN, policy.DN)

	t.Run("inside the duration", func(t *testing.T) {
		writeLockedTime(t, env, user.DN, ldap.GeneralizedTime(time.Now().Add(-time.Minute)))

		got := getUser(t, env, token, user.DN)
		if got.Lockout == nil {
			t.Fatal("no lockout resolved")
		}
		if got.Lockout.State != ldap.LockStateActive {
			t.Errorf("state = %q, want %q", got.Lockout.State, ldap.LockStateActive)
		}
		if got.Lockout.LiftsAt == "" {
			t.Error("no lift instant for a lockout with a duration")
		}
		if got.Lockout.DurationSeconds != 900 {
			t.Errorf("durationSeconds = %d, want 900", got.Lockout.DurationSeconds)
		}
		if !got.Lockout.Locked() {
			t.Error("a lockout inside its duration does not report as locked")
		}
	})

	t.Run("past the duration", func(t *testing.T) {
		writeLockedTime(t, env, user.DN, ldap.GeneralizedTime(time.Now().Add(-2*time.Hour)))

		got := getUser(t, env, token, user.DN)
		if got.Lockout == nil {
			t.Fatal("no lockout resolved")
		}
		if got.Lockout.State != ldap.LockStateLifted {
			t.Fatalf("state = %q, want %q", got.Lockout.State, ldap.LockStateLifted)
		}
		if got.Lockout.Locked() {
			t.Error("a lapsed lockout still reports as locked")
		}
		// The attribute is deliberately still reported: it is on the entry, the
		// Unlock button clears it, and anything reading the directory directly sees
		// it until the next bind.
		if !got.AccountLocked || got.PwdAccountLockedTime == "" {
			t.Error("the lock attribute went missing from the response")
		}
	})

	t.Run("a manual lock never lapses", func(t *testing.T) {
		if resp, body := doJSON(t, env, http.MethodPost, userPath(user.DN)+"/lock", nil, token); resp.StatusCode != http.StatusOK {
			t.Fatalf("lock: status=%d body=%s", resp.StatusCode, body)
		}

		got := getUser(t, env, token, user.DN)
		if got.Lockout == nil || got.Lockout.State != ldap.LockStateManual {
			t.Fatalf("state = %v, want %q", got.Lockout, ldap.LockStateManual)
		}
		if !got.Lockout.Locked() {
			t.Error("a manual lock does not report as locked")
		}
	})

	t.Run("no policy leaves the answer unknown", func(t *testing.T) {
		// Drop the entry's own policy: with nothing left but olcPPolicyDefault,
		// which no ordinary bind identity can read, the duration is unknowable
		// unless LDAP_DEFAULT_POLICY_DN says what it is.
		if env.Config.LDAP.DefaultPolicyDN != "" {
			t.Skip("LDAP_DEFAULT_POLICY_DN is set, so the duration is known")
		}
		applyPolicy(t, env, token, user.DN, "")
		writeLockedTime(t, env, user.DN, ldap.GeneralizedTime(time.Now().Add(-2*time.Hour)))

		got := getUser(t, env, token, user.DN)
		if got.Lockout == nil || got.Lockout.State != ldap.LockStateUnknown {
			t.Fatalf("state = %v, want %q", got.Lockout, ldap.LockStateUnknown)
		}
		if !got.Lockout.Locked() {
			t.Error("an unresolved lock reports the account as usable")
		}
	})
}

// TestIntegration_LockoutDefaultPolicy covers the case most entries are in: no
// pwdPolicySubentry of their own, so the duration comes from the policy in
// olcPPolicyDefault — which slapd will not show us, and which
// LDAP_DEFAULT_POLICY_DN therefore has to declare.
func TestIntegration_LockoutDefaultPolicy(t *testing.T) {
	// The bundled stack's default, wired up in ldap/config and seed.ldif.
	const defaultPolicyDN = "cn=default,ou=policies,dc=example,dc=org"
	t.Setenv("LDAP_DEFAULT_POLICY_DN", defaultPolicyDN)

	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	policy, err := env.LDAP.GetPasswordPolicy(defaultPolicyDN)
	if err != nil {
		t.Skipf("no %s in this directory: %v", defaultPolicyDN, err)
	}
	if policy.PwdLockoutDuration <= 0 {
		t.Skipf("%s has no pwdLockoutDuration, so nothing lifts on its own", defaultPolicyDN)
	}
	duration := time.Duration(policy.PwdLockoutDuration) * time.Second

	user := createTestUser(t, env, token)
	writeLockedTime(t, env, user.DN, ldap.GeneralizedTime(time.Now().Add(-2*duration)))

	got := getUser(t, env, token, user.DN)
	if got.Lockout == nil {
		t.Fatal("no lockout resolved")
	}
	if got.Lockout.State != ldap.LockStateLifted {
		t.Errorf("state = %q, want %q — the declared default policy was not applied",
			got.Lockout.State, ldap.LockStateLifted)
	}
	if got.Lockout.PolicyDN != defaultPolicyDN {
		t.Errorf("policyDn = %q, want %q", got.Lockout.PolicyDN, defaultPolicyDN)
	}

	// Same entry, same policy, a lock that has not run out yet.
	writeLockedTime(t, env, user.DN, ldap.GeneralizedTime(time.Now()))
	if got := getUser(t, env, token, user.DN); got.Lockout.State != ldap.LockStateActive {
		t.Errorf("state = %q, want %q", got.Lockout.State, ldap.LockStateActive)
	}
}

// TestIntegration_LockoutStateInList checks the listing and the dashboard agree
// with the user's own page. They answer from different places — the list resolves
// each row, the counts come from an LDAP filter — and a lapsed lockout showing as
// locked in one and active in the other is the same lie in a different corner of
// the UI.
func TestIntegration_LockoutStateInList(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token
	user := createTestUser(t, env, token)
	policy := createLockoutPolicy(t, env, 900)
	applyPolicy(t, env, token, user.DN, policy.DN)
	writeLockedTime(t, env, user.DN, ldap.GeneralizedTime(time.Now().Add(-2*time.Hour)))

	lockedBefore := overviewLockedCount(t, env, token)

	item := findUserInList(t, env, token, user.UID, "")
	if item.Lockout == nil || item.Lockout.State != ldap.LockStateLifted {
		t.Fatalf("row lockout = %v, want %q", item.Lockout, ldap.LockStateLifted)
	}

	// The status filter follows the resolved state: the account authenticates, so
	// it belongs with the active ones.
	if findUserInList(t, env, token, user.UID, "&status=active") == nil {
		t.Error("a user whose lockout lapsed is missing from status=active")
	}
	if findUserInList(t, env, token, user.UID, "&status=inactive") != nil {
		t.Error("a user whose lockout lapsed is still listed as inactive")
	}

	// And the dashboard counts the same way, which needs the lock's policy to be
	// reachable from a filter: the entry carries its own pwdPolicySubentry here.
	if lockedBefore != 0 {
		t.Logf("locked count = %d; other locked accounts exist in the directory", lockedBefore)
	}

	// Make the lock current again and the figure must move by one.
	writeLockedTime(t, env, user.DN, ldap.GeneralizedTime(time.Now()))
	if lockedNow := overviewLockedCount(t, env, token); lockedNow != lockedBefore+1 {
		t.Errorf("locked count = %d after a live lockout, want %d", lockedNow, lockedBefore+1)
	}
}

// overviewLockedCount reads the dashboard's locked figure.
func overviewLockedCount(t *testing.T, env *testEnv, token string) int {
	t.Helper()
	resp, body := doJSON(t, env, http.MethodGet, "/api/overview", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("overview: status=%d body=%s", resp.StatusCode, body)
	}
	var overview struct {
		Users struct {
			Locked int `json:"locked"`
		} `json:"users"`
	}
	if err := json.Unmarshal(body, &overview); err != nil {
		t.Fatalf("unmarshal overview: %v", err)
	}
	return overview.Users.Locked
}

// findUserInList returns the row for a uid from the users listing, or nil when
// the filters exclude it.
func findUserInList(t *testing.T, env *testEnv, token, uid, extraQuery string) *ldap.UserListItem {
	t.Helper()
	path := fmt.Sprintf("/api/users/?search=%s%s", uid, extraQuery)
	resp, body := doJSON(t, env, http.MethodGet, path, nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list users: status=%d body=%s", resp.StatusCode, body)
	}
	var page struct {
		Data []ldap.UserListItem `json:"data"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("unmarshal users: %v", err)
	}
	for i := range page.Data {
		if page.Data[i].UID == uid {
			return &page.Data[i]
		}
	}
	return nil
}
