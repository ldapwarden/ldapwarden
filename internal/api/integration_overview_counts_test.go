//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// TestIntegration_Overview_CountsAreNotSelfTruncated pins that the dashboard no
// longer bounds its own counts by LDAP_SEARCH_SIZE_LIMIT.
//
// It used to compute every figure by fetching all user entries with the list
// projection, capped at that limit, and then warn that the numbers were lower
// bounds. On any directory past the cap the warning was permanent and named a knob
// the operator had no reason to touch — the limit exists to keep a list view's
// payload small, not to bound a count.
func TestIntegration_Overview_CountsAreNotSelfTruncated(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	overview := getOverview(t, env, token)

	// The seeded directory is far below any plausible limit, so nothing may claim
	// to have been cut short.
	if overview.Truncated {
		t.Error("counts reported as truncated on a directory of six users")
	}

	// Cross-check the total against the list endpoint, which counts the same
	// population by a different route.
	resp, body := doJSON(t, env, http.MethodGet, "/api/users?limit=1", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list users: status=%d body=%s", resp.StatusCode, body)
	}
	var list struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if overview.Users.Total != list.Total {
		t.Errorf("overview counts %d users, the list endpoint counts %d",
			overview.Users.Total, list.Total)
	}
}

// TestIntegration_Overview_ActiveExcludesEachAccountOnce is the arithmetic that a
// count-per-state approach can get wrong: an account that is both locked and
// expired must be subtracted once, not twice.
func TestIntegration_Overview_ActiveExcludesEachAccountOnce(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token
	if !env.LDAP.Capabilities().ScheduledExpiry {
		t.Skip("directory has no pwdEndTime; an account cannot be both locked and expired")
	}

	before := getOverview(t, env, token)

	// One account locked, one expired, one both.
	locked := createTestUser(t, env, token)
	expired := createTestUser(t, env, token)
	both := createTestUser(t, env, token)

	past := time.Now().AddDate(0, 0, -1).Format(time.DateOnly)
	for _, u := range []*ldap.User{locked, both} {
		if resp, body := doJSON(t, env, http.MethodPost, userPath(u.DN)+"/lock", nil, token); resp.StatusCode != http.StatusOK {
			t.Fatalf("lock %s: status=%d body=%s", u.UID, resp.StatusCode, body)
		}
	}
	for _, u := range []*ldap.User{expired, both} {
		if resp, body := doJSON(t, env, http.MethodPut, userPath(u.DN)+"/expiration",
			map[string]string{"expirationDate": past}, token); resp.StatusCode != http.StatusOK {
			t.Fatalf("expire %s: status=%d body=%s", u.UID, resp.StatusCode, body)
		}
	}

	after := getOverview(t, env, token)

	if got, want := after.Users.Total-before.Users.Total, 3; got != want {
		t.Fatalf("total grew by %d, want %d", got, want)
	}
	if got, want := after.Users.Locked-before.Users.Locked, 2; got != want {
		t.Errorf("locked grew by %d, want %d", got, want)
	}
	if got, want := after.Users.Expired-before.Users.Expired, 2; got != want {
		t.Errorf("expired grew by %d, want %d", got, want)
	}
	// Three accounts were added and all three are inactive, so active must not
	// move. Subtracting locked+expired would have it fall by one.
	if got := after.Users.Active - before.Users.Active; got != 0 {
		t.Errorf("active moved by %d after adding three inactive accounts, want 0 — "+
			"an account that is both locked and expired is being counted twice", got)
	}
}
