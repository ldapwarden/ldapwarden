//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

func getOverview(t *testing.T, env *testEnv, token string) ldap.Overview {
	t.Helper()
	resp, body := doJSON(t, env, http.MethodGet, "/api/overview/", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get overview: status=%d body=%s", resp.StatusCode, body)
	}
	var out ldap.Overview
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal overview: %v (body=%s)", err, body)
	}
	return out
}

// TestIntegration_Overview_CountsAndBuckets checks the dashboard figures hang
// together and that locking an account moves it out of the active bucket.
func TestIntegration_Overview_CountsAndBuckets(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	before := getOverview(t, env, token)
	// Not "the buckets add up": locked and expired are independent states, so an
	// account that is both is in each and is excluded from active once. Active is
	// the complement of the union, which is what these two assertions pin.
	if before.Users.Active > before.Users.Total {
		t.Errorf("more active accounts than accounts: %+v", before.Users)
	}
	if inactive := before.Users.Total - before.Users.Active; before.Users.Locked+before.Users.Expired < inactive {
		t.Errorf("locked+expired (%d) is below the number of inactive accounts (%d): %+v",
			before.Users.Locked+before.Users.Expired, inactive, before.Users)
	}
	if before.Groups < 1 {
		t.Errorf("groups = %d, want at least the seeded groups", before.Groups)
	}

	user := createTestUser(t, env, token)

	created := getOverview(t, env, token)
	if created.Users.Total != before.Users.Total+1 {
		t.Errorf("total after create = %d, want %d", created.Users.Total, before.Users.Total+1)
	}
	if created.Users.Active != before.Users.Active+1 {
		t.Errorf("active after create = %d, want %d", created.Users.Active, before.Users.Active+1)
	}

	if resp, body := doJSON(t, env, http.MethodPost, userPath(user.DN)+"/lock", nil, token); resp.StatusCode != http.StatusOK {
		t.Fatalf("lock user: status=%d body=%s", resp.StatusCode, body)
	}

	locked := getOverview(t, env, token)
	if locked.Users.Locked != created.Users.Locked+1 {
		t.Errorf("locked after lock = %d, want %d", locked.Users.Locked, created.Users.Locked+1)
	}
	if locked.Users.Active != created.Users.Active-1 {
		t.Errorf("active after lock = %d, want %d", locked.Users.Active, created.Users.Active-1)
	}
	if locked.Users.Total != created.Users.Total {
		t.Errorf("locking changed the total: %d -> %d", created.Users.Total, locked.Users.Total)
	}
}

// TestIntegration_Overview_ExpiringWindow checks the expiring list picks up an
// account inside the 30-day window and ignores one beyond it — the dashboard's
// whole purpose is that this list is neither empty nor noisy.
func TestIntegration_Overview_ExpiringWindow(t *testing.T) {
	env := setupTestServer(t)
	if !env.LDAP.Capabilities().ScheduledExpiry {
		t.Skip("directory has no pwdEndTime, so no account can carry an expiry to report")
	}
	token := loginAs(t, env, "admin", "admin123").Token

	soon := createTestUser(t, env, token)
	later := createTestUser(t, env, token)

	setExpiration := func(dn, date string) {
		t.Helper()
		resp, body := doJSON(t, env, http.MethodPut, userPath(dn)+"/expiration",
			map[string]string{"expirationDate": date}, token)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("set expiration on %s: status=%d body=%s", dn, resp.StatusCode, body)
		}
	}

	inWindow := time.Now().AddDate(0, 0, 10).Format(time.DateOnly)
	beyond := time.Now().AddDate(0, 0, 120).Format(time.DateOnly)
	setExpiration(soon.DN, inWindow)
	setExpiration(later.DN, beyond)

	overview := getOverview(t, env, token)

	var foundSoon, foundLater bool
	for _, account := range overview.Users.Expiring {
		if account.DN == soon.DN {
			foundSoon = true
		}
		if account.DN == later.DN {
			foundLater = true
		}
	}
	if !foundSoon {
		t.Errorf("account expiring in 10 days missing from the expiring list: %+v", overview.Users.Expiring)
	}
	if foundLater {
		t.Error("account expiring in 120 days should be outside the 30-day window")
	}

	// Soonest first, and never more than the display cap.
	if len(overview.Users.Expiring) > 5 {
		t.Errorf("expiring list not capped: %d entries", len(overview.Users.Expiring))
	}
	for i := 1; i < len(overview.Users.Expiring); i++ {
		if overview.Users.Expiring[i].ExpiresAt.Before(overview.Users.Expiring[i-1].ExpiresAt) {
			t.Errorf("expiring list is not soonest-first: %+v", overview.Users.Expiring)
			break
		}
	}
}

// TestIntegration_Overview_ReadOnlyAllowed keeps the dashboard reachable for a
// read-only operator: the figures aggregate data they can already list.
func TestIntegration_Overview_ReadOnlyAllowed(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "viewer", "viewer123").Token

	overview := getOverview(t, env, token)
	if overview.Users.Total < 1 {
		t.Errorf("read-only overview looks empty: %+v", overview)
	}
}
