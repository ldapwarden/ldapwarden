//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// getUser fetches a single user through the API.
func getUser(t *testing.T, env *testEnv, token, dn string) ldap.User {
	t.Helper()
	resp, body := doJSON(t, env, http.MethodGet, userPath(dn), nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get user: status=%d body=%s", resp.StatusCode, body)
	}
	var user ldap.User
	if err := json.Unmarshal(body, &user); err != nil {
		t.Fatalf("unmarshal user: %v", err)
	}
	return user
}

// TestIntegration_Lock_IsPermanent checks a lock stays a lock.
//
// It used to write the current time, which OpenLDAP reads as "a lockout started
// now" and lifts once pwdLockoutDuration elapses — so "Locked" in the UI became
// "unlocked" half an hour later, with nothing recorded. The permanent-lock value
// has no such window.
func TestIntegration_Lock_IsPermanent(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token
	user := createTestUser(t, env, token)

	if resp, body := doJSON(t, env, http.MethodPost, userPath(user.DN)+"/lock", nil, token); resp.StatusCode != http.StatusOK {
		t.Fatalf("lock: status=%d body=%s", resp.StatusCode, body)
	}

	locked := getUser(t, env, token, user.DN)
	if !locked.AccountLocked {
		t.Error("account not reported as locked")
	}
	// The value must be one ppolicy never lifts, not a timestamp.
	if _, err := time.Parse("20060102150405Z", locked.PwdAccountLockedTime); err == nil {
		t.Errorf("lock wrote a concrete timestamp (%s): ppolicy will lift it after pwdLockoutDuration",
			locked.PwdAccountLockedTime)
	}

	// Locking must not invent an expiry.
	if locked.PwdEndTime != "" {
		t.Errorf("lock also set an expiry: pwdEndTime=%s", locked.PwdEndTime)
	}

	if resp, body := doJSON(t, env, http.MethodPost, userPath(user.DN)+"/unlock", nil, token); resp.StatusCode != http.StatusOK {
		t.Fatalf("unlock: status=%d body=%s", resp.StatusCode, body)
	}
	unlocked := getUser(t, env, token, user.DN)
	if unlocked.AccountLocked || unlocked.PwdAccountLockedTime != "" {
		t.Errorf("still locked after unlock: %+v", unlocked.PwdAccountLockedTime)
	}
}

// TestIntegration_ScheduledExpiry covers the two directory generations from one
// test, because which branch runs is a property of the server, not the code:
// with pwdEndTime the expiry is stored and enforced there, without it the request
// is refused instead of writing an attribute the server ignores.
func TestIntegration_ScheduledExpiry(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token
	user := createTestUser(t, env, token)

	supported := env.LDAP.Capabilities().ScheduledExpiry
	future := time.Now().AddDate(0, 0, 30).Format(time.DateOnly)

	resp, body := doJSON(t, env, http.MethodPut, userPath(user.DN)+"/expiration",
		map[string]string{"expirationDate": future}, token)

	if !supported {
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("expiry on a directory without pwdEndTime: status=%d body=%s, want 422", resp.StatusCode, body)
		}
		t.Logf("directory has no pwdEndTime; refusal path verified")
		return
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set expiration: status=%d body=%s", resp.StatusCode, body)
	}

	expiring := getUser(t, env, token, user.DN)
	if expiring.PwdEndTime == "" {
		t.Fatal("expiration was not stored in pwdEndTime")
	}
	// The lock attribute must stay out of it: writing a future value there is
	// what disabled accounts the moment they were scheduled.
	if expiring.PwdAccountLockedTime != "" {
		t.Errorf("scheduling an expiry touched the lock attribute: %s", expiring.PwdAccountLockedTime)
	}
	if expiring.AccountLocked {
		t.Error("account reported as locked merely for having a future expiry")
	}

	// Clearing works and leaves nothing behind.
	if resp, body := doJSON(t, env, http.MethodPut, userPath(user.DN)+"/expiration",
		map[string]string{"expirationDate": ""}, token); resp.StatusCode != http.StatusOK {
		t.Fatalf("clear expiration: status=%d body=%s", resp.StatusCode, body)
	}
	if cleared := getUser(t, env, token, user.DN); cleared.PwdEndTime != "" {
		t.Errorf("expiry still set after clearing: %s", cleared.PwdEndTime)
	}
}

// TestIntegration_Overview_ReportsCapability pins that the UI can find out
// whether to offer scheduled expiry at all.
func TestIntegration_Overview_ReportsCapability(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	resp, body := doJSON(t, env, http.MethodGet, "/api/admin/config", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get config: status=%d body=%s", resp.StatusCode, body)
	}
	var config struct {
		Capabilities struct {
			ScheduledExpirySupported bool   `json:"scheduledExpirySupported"`
			PolicyEnforcement        string `json:"policyEnforcement"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(body, &config); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if config.Capabilities.ScheduledExpirySupported != env.LDAP.Capabilities().ScheduledExpiry {
		t.Errorf("config reports scheduledExpirySupported=%v, directory probe says %v",
			config.Capabilities.ScheduledExpirySupported, env.LDAP.Capabilities().ScheduledExpiry)
	}
	// Whatever the answer, it has to be one of the three — the UI switches on it.
	switch config.Capabilities.PolicyEnforcement {
	case "enforced", "not-enforced", "unknown":
	default:
		t.Errorf("policyEnforcement=%q is not one of enforced/not-enforced/unknown",
			config.Capabilities.PolicyEnforcement)
	}
}

// TestIntegration_ExpiryMigration_DryRunWritesNothing is the guard that matters
// most on a migration: the default invocation must not touch the directory.
func TestIntegration_ExpiryMigration_DryRunWritesNothing(t *testing.T) {
	env := setupTestServer(t)
	if !env.LDAP.Capabilities().ScheduledExpiry {
		t.Skip("directory has no pwdEndTime; nothing to migrate onto")
	}
	token := loginAs(t, env, "admin", "admin123").Token
	user := createTestUser(t, env, token)

	// Recreate the legacy encoding: an expiry parked in the lock attribute.
	future := time.Now().AddDate(0, 0, 45).UTC().Format("20060102150405Z")
	if err := env.LDAP.SetRawAttribute(user.DN, "pwdAccountLockedTime", future); err != nil {
		t.Fatalf("seed legacy expiry: %v", err)
	}

	report, err := env.LDAP.MigrateAccountExpiry(ldap.ExpiryMigrationOptions{})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if report.Applied {
		t.Error("dry run reported itself as applied")
	}

	var found bool
	for _, entry := range report.Entries {
		if entry.DN == user.DN {
			found = true
			if entry.Action != "migrate" {
				t.Errorf("future lock timestamp: action=%q, want migrate", entry.Action)
			}
		}
	}
	if !found {
		t.Error("the seeded account was not reported")
	}

	// Nothing may have changed on disk.
	after := getUser(t, env, token, user.DN)
	if after.PwdEndTime != "" {
		t.Errorf("dry run wrote pwdEndTime=%s", after.PwdEndTime)
	}
	if after.PwdAccountLockedTime != future {
		t.Errorf("dry run altered the lock attribute: %s", after.PwdAccountLockedTime)
	}

	// And applying moves it across, clearing the attribute that disabled it.
	if _, err := env.LDAP.MigrateAccountExpiry(ldap.ExpiryMigrationOptions{Apply: true}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	migrated := getUser(t, env, token, user.DN)
	if migrated.PwdEndTime != future {
		t.Errorf("pwdEndTime=%q, want %q", migrated.PwdEndTime, future)
	}
	if migrated.PwdAccountLockedTime != "" {
		t.Errorf("lock attribute left behind: %s", migrated.PwdAccountLockedTime)
	}
}
