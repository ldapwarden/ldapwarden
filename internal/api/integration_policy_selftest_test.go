//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

func runSelfTest(t *testing.T, env *testEnv, token string) ldap.PolicySelfTest {
	t.Helper()
	resp, body := doJSON(t, env, http.MethodPost, "/api/admin/policy-selftest", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("policy self-test: status=%d body=%s", resp.StatusCode, body)
	}
	var result ldap.PolicySelfTest
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal self-test: %v", err)
	}
	return result
}

// TestIntegration_PolicySelfTest_ReachesAVerdictAndCleansUp is the test that
// matters: the self-test writes to the directory, so it has to take its entry
// away again whatever it concludes.
//
// The verdict itself depends on the server under test, which is the point — no
// read can tell you whether a policy applies (cn=config is hidden from an
// ordinary bind identity, and every other signal is identical either way), so
// this is the only thing that can answer.
func TestIntegration_PolicySelfTest_ReachesAVerdictAndCleansUp(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	result := runSelfTest(t, env, token)

	switch result.Result {
	case ldap.EnforcementEnforced, ldap.EnforcementNotEnforced:
	case ldap.EnforcementUnknown:
		t.Logf("inconclusive, which is a legitimate outcome: %s", result.Detail)
	default:
		t.Errorf("result=%q is not one of the three states", result.Result)
	}

	if !result.CleanedUp {
		t.Errorf("the probe entry was left behind at %s: %v", result.ProbeDN, result.Steps)
	}
	if len(result.Steps) == 0 {
		t.Error("no steps recorded: the verdict cannot be checked")
	}

	// And the entry really is gone, not merely reported as gone.
	if resp, _ := doJSON(t, env, http.MethodGet, userPath(result.ProbeDN), nil, token); resp.StatusCode != http.StatusNotFound {
		t.Errorf("probe entry still readable: status=%d", resp.StatusCode)
	}

	// The bundled directory has olcPPolicyDefault wired up, so on it the answer
	// must be "enforced" — otherwise locking silently does nothing and every
	// lock-related test in this suite is meaningless.
	if env.LDAP.Capabilities().ScheduledExpiry && result.Result != ldap.EnforcementEnforced {
		t.Errorf("the test directory does not enforce its policy (result=%q): %s",
			result.Result, result.Detail)
	}
}

// TestIntegration_PolicySelfTest_RefusesToTouchAnExistingEntry guards the one way
// this feature could destroy data: reusing a DN somebody else owns.
func TestIntegration_PolicySelfTest_RefusesToTouchAnExistingEntry(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	probeDN := "uid=" + ldap.PolicySelfTestUID + "," + env.LDAP.UserBaseDN()
	resp, body := doJSON(t, env, http.MethodPost, "/api/users/", map[string]any{
		"uid":         ldap.PolicySelfTestUID,
		"givenName":   "Not",
		"sn":          "Yours",
		"uidNumber":   60999,
		"gidNumber":   60999,
		"description": "planted by a test",
	}, token)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("plant entry: status=%d body=%s", resp.StatusCode, body)
	}
	t.Cleanup(func() {
		_, _ = doJSON(t, env, http.MethodDelete, userPath(probeDN), nil, token)
	})

	if resp, body := doJSON(t, env, http.MethodPost, "/api/admin/policy-selftest", nil, token); resp.StatusCode != http.StatusConflict {
		t.Fatalf("self-test with the DN taken: status=%d body=%s, want 409", resp.StatusCode, body)
	}

	// Untouched: same description, still there.
	resp, body = doJSON(t, env, http.MethodGet, userPath(probeDN), nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("planted entry disappeared: status=%d body=%s", resp.StatusCode, body)
	}
	var user ldap.User
	if err := json.Unmarshal(body, &user); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if user.Description != "planted by a test" {
		t.Errorf("planted entry was modified: description=%q", user.Description)
	}
}

// TestIntegration_PolicySelfTest_VerdictSurfacesInCapabilities pins the wiring:
// once something has been proven, the app must stop answering "unknown".
func TestIntegration_PolicySelfTest_VerdictSurfacesInCapabilities(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	result := runSelfTest(t, env, token)
	if result.Result == ldap.EnforcementUnknown {
		t.Skip("self-test was inconclusive; nothing to surface")
	}

	resp, body := doJSON(t, env, http.MethodGet, "/api/admin/config", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get config: status=%d body=%s", resp.StatusCode, body)
	}
	var config struct {
		Capabilities struct {
			PolicyEnforcement string `json:"policyEnforcement"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(body, &config); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if config.Capabilities.PolicyEnforcement != string(result.Result) {
		t.Errorf("capabilities report %q after a self-test that concluded %q",
			config.Capabilities.PolicyEnforcement, result.Result)
	}
}
