//go:build integration

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// lockAccountByFailedBinds locks an account the way it happens in production: by
// getting the password wrong until ppolicy gives up on it.
//
// Nothing else can produce this state. pwdFailureTime is NO-USER-MODIFICATION —
// slapd refuses to let even the rootdn write it — so the timestamps this feature
// exists to report can only come from real failed binds.
func lockAccountByFailedBinds(t *testing.T, env *testEnv, dn string, attempts int) {
	t.Helper()
	for i := 0; i < attempts; i++ {
		if err := env.LDAP.Bind(dn, "definitely-not-the-password"); err == nil {
			t.Fatalf("bind %d with a wrong password succeeded", i+1)
		}
	}
}

// triggerLockoutScan runs the scan through its endpoint and returns the result.
func triggerLockoutScan(t *testing.T, env *testEnv, token string) TriggerTaskResponse {
	t.Helper()
	resp, body := doJSON(t, env, http.MethodPost,
		"/api/admin/scheduled-tasks/lockout-scan/trigger", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("trigger lockout scan: status=%d body=%s", resp.StatusCode, body)
	}
	var out TriggerTaskResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal trigger response: %v (body=%s)", err, body)
	}
	if len(out.Errors) > 0 {
		// Not a failure: a run reports a missing mail server or a slow schedule this
		// way, and the audit trail is written regardless. Logged so a broken
		// assertion below can be read against it.
		t.Logf("scan reported: %v", out.Errors)
	}
	return out
}

// lockoutAuditRows returns the recorded lockouts for one entry, newest first,
// with their details. Read from the database rather than the API because the
// details JSON — where the failed-bind timestamps live — is the point.
func lockoutAuditRows(t *testing.T, env *testEnv, dn string) []map[string]any {
	t.Helper()
	rows, err := env.Pool.Query(context.Background(), `
		SELECT details FROM audit_logs
		WHERE action = 'user.lockout' AND resource_dn = $1
		ORDER BY created_at DESC
	`, dn)
	if err != nil {
		t.Fatalf("query audit rows: %v", err)
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatalf("scan audit row: %v", err)
		}
		var details map[string]any
		if err := json.Unmarshal(raw, &details); err != nil {
			t.Fatalf("unmarshal audit details: %v", err)
		}
		out = append(out, details)
	}
	return out
}

// TestIntegration_LockoutScan is the whole point of the feature: an account that
// locks itself gets reported, once, with the evidence.
//
// The evidence is perishable — ppolicy deletes pwdFailureTime and
// pwdAccountLockedTime at the account's next sign-in — so the assertion that
// matters is that the audit row holds the timestamps, not that the directory does.
func TestIntegration_LockoutScan(t *testing.T) {
	// Declaring the default policy lets the scan resolve the lockout window; without
	// it the lockout is still reported, just as "unknown".
	t.Setenv("LDAP_DEFAULT_POLICY_DN", "cn=default,ou=policies,dc=example,dc=org")

	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	policy, err := env.LDAP.GetPasswordPolicy("cn=default,ou=policies,dc=example,dc=org")
	if err != nil {
		t.Skipf("no default policy in this directory: %v", err)
	}
	if !policy.PwdLockout || policy.PwdMaxFailure <= 0 {
		t.Skip("the default policy does not lock accounts, so nothing can lock itself")
	}

	user := createTestUser(t, env, token)

	// A clean run first: whatever else is locked in the directory, this entry is not,
	// so it must not appear.
	triggerLockoutScan(t, env, token)
	if rows := lockoutAuditRows(t, env, user.DN); len(rows) != 0 {
		t.Fatalf("an unlocked account was reported as locked: %v", rows)
	}

	lockAccountByFailedBinds(t, env, user.DN, policy.PwdMaxFailure)

	locked := getUser(t, env, token, user.DN)
	if locked.PwdAccountLockedTime == "" {
		t.Skipf("the directory did not lock the account after %d failed binds: ppolicy is not enforcing here",
			policy.PwdMaxFailure)
	}

	result := triggerLockoutScan(t, env, token)
	if result.UsersChecked < 1 {
		t.Errorf("usersChecked = %d, want at least 1", result.UsersChecked)
	}

	rows := lockoutAuditRows(t, env, user.DN)
	if len(rows) != 1 {
		t.Fatalf("audit rows = %d, want exactly 1: %v", len(rows), rows)
	}
	details := rows[0]

	if details["userUid"] != user.UID {
		t.Errorf("userUid = %v, want %q", details["userUid"], user.UID)
	}
	if details["lockedAt"] != locked.PwdAccountLockedTime {
		t.Errorf("lockedAt = %v, want %q", details["lockedAt"], locked.PwdAccountLockedTime)
	}

	// The failed binds, copied out of the entry while they still existed.
	failed, ok := details["failedBinds"].([]any)
	if !ok || len(failed) == 0 {
		t.Fatalf("failedBinds = %v, want the recorded attempt timestamps", details["failedBinds"])
	}
	for _, attempt := range failed {
		if _, err := time.Parse(time.RFC3339, attempt.(string)); err != nil {
			t.Errorf("failedBinds entry %q is not a timestamp: %v", attempt, err)
		}
	}
	// With the policy declared, the window is resolved rather than unknown.
	if details["liftsAt"] == nil {
		t.Error("no liftsAt recorded although the applicable policy states a duration")
	}
	if details["lockState"] != "active" {
		t.Errorf("lockState = %v, want active", details["lockState"])
	}

	// Running again must not report the same lockout twice: the account is still
	// locked, and a scan every five minutes would otherwise mail about it all day.
	triggerLockoutScan(t, env, token)
	if rows := lockoutAuditRows(t, env, user.DN); len(rows) != 1 {
		t.Errorf("audit rows = %d after a second scan, want 1 — the same lockout was reported twice", len(rows))
	}

	// Unlocking and locking again is a new event, and has to be reported as one.
	if resp, body := doJSON(t, env, http.MethodPost, userPath(user.DN)+"/unlock", nil, token); resp.StatusCode != http.StatusOK {
		t.Fatalf("unlock: status=%d body=%s", resp.StatusCode, body)
	}
	triggerLockoutScan(t, env, token)

	// Past the current second before locking again: pwdAccountLockedTime has
	// second granularity, so two lockouts inside one second are the same event as
	// far as anything reading the directory can tell — including this test.
	time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(1100 * time.Millisecond)))
	lockAccountByFailedBinds(t, env, user.DN, policy.PwdMaxFailure)

	relocked := getUser(t, env, token, user.DN)
	if relocked.PwdAccountLockedTime == locked.PwdAccountLockedTime {
		t.Fatalf("the second lockout carries the first one's timestamp (%s), so the wait did not work",
			relocked.PwdAccountLockedTime)
	}
	triggerLockoutScan(t, env, token)
	if rows := lockoutAuditRows(t, env, user.DN); len(rows) != 2 {
		t.Errorf("audit rows = %d after a second lockout, want 2", len(rows))
	}
}

// TestIntegration_LockoutScanIgnoresManualLocks keeps the two kinds of lock apart.
// An administrator locking an account here is already audited and emailed where it
// happens; reporting it again as a directory lockout would be a false alarm about
// an attack that never occurred.
func TestIntegration_LockoutScanIgnoresManualLocks(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token
	user := createTestUser(t, env, token)

	if resp, body := doJSON(t, env, http.MethodPost, userPath(user.DN)+"/lock", nil, token); resp.StatusCode != http.StatusOK {
		t.Fatalf("lock: status=%d body=%s", resp.StatusCode, body)
	}

	triggerLockoutScan(t, env, token)

	if rows := lockoutAuditRows(t, env, user.DN); len(rows) != 0 {
		t.Errorf("an administrator's lock was reported as a directory lockout: %v", rows)
	}
}
