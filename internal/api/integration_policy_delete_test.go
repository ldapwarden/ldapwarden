//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

func policyPath(dn string) string {
	return "/api/password-policies/" + url.PathEscape(dn)
}

// TestIntegration_PasswordPolicy_Delete_ReportsLeftovers: pwdPolicySubentry is
// a DN the directory does not follow, so an account naming a deleted policy
// stops getting it in silence. The delete goes through; the account must come
// back as a warning.
func TestIntegration_PasswordPolicy_Delete_ReportsLeftovers(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	cn := "zz-delpol-" + uniqueSuffix(t)
	resp, body := doJSON(t, env, http.MethodPost, "/api/password-policies/", map[string]any{
		"cn": cn, "pwdAttribute": "userPassword",
	}, token)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create policy: status=%d body=%s", resp.StatusCode, body)
	}
	var policy ldap.PasswordPolicy
	if err := json.Unmarshal(body, &policy); err != nil {
		t.Fatalf("unmarshal policy: %v", err)
	}
	t.Cleanup(func() {
		_, _ = doJSON(t, env, http.MethodDelete, policyPath(policy.DN), nil, token)
	})

	user := createTestUser(t, env, token)
	resp, body = doJSON(t, env, http.MethodPut, userPath(user.DN),
		map[string]any{"pwdPolicySubentry": policy.DN}, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("assign policy: status=%d body=%s", resp.StatusCode, body)
	}

	resp, body = doJSON(t, env, http.MethodDelete, policyPath(policy.DN), nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete: status=%d body=%s", resp.StatusCode, body)
	}
	var result struct {
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, body)
	}
	if !strings.Contains(strings.Join(result.Warnings, "\n"), user.UID) {
		t.Errorf("warnings do not name %q, which still points at the policy: %v", user.UID, result.Warnings)
	}
}
