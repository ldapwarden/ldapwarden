//go:build integration

package api

import (
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// TestEffectivePasswordPolicy covers the resolution behind the password-rejection
// message and the requirements panel: the entry's own pwdPolicySubentry, else the
// directory default declared in LDAP_DEFAULT_POLICY_DN. Both branches have to be
// exercised against a real directory, because the whole point of the function is
// that the answer is not inferable — olcPPolicyDefault is not readable, and a
// wrong policy would state rules the directory does not impose.
func TestEffectivePasswordPolicy(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token
	user := createTestUser(t, env, token)

	t.Run("falls back to the configured directory default", func(t *testing.T) {
		if env.Config.LDAP.DefaultPolicyDN == "" {
			t.Skip("LDAP_DEFAULT_POLICY_DN not configured for this run")
		}
		policy := env.LDAP.EffectivePasswordPolicy(user.DN)
		if policy == nil {
			t.Fatal("no policy resolved for an entry with no pwdPolicySubentry, want the default")
		}
		// The bundled default policy is the one LDAP_DEFAULT_POLICY_DN names.
		if policy.PwdMinLength <= 0 {
			t.Errorf("resolved policy %q has no pwdMinLength; it would name no actionable rule", policy.CN)
		}
		if got := passwordRules(policy); got == "" {
			t.Error("passwordRules() is empty for the default policy, so a rejection would explain nothing")
		} else {
			t.Logf("default policy sentence: %s", got)
		}
	})

	t.Run("prefers the entry's own subentry", func(t *testing.T) {
		policies, err := env.LDAP.ListPasswordPolicies()
		if err != nil {
			t.Fatalf("list policies: %v", err)
		}
		// Pick a policy that is not the configured default, so the assertion
		// cannot pass by accident.
		var other *ldap.PasswordPolicy
		for i := range policies {
			if policies[i].DN != env.Config.LDAP.DefaultPolicyDN {
				other = &policies[i]
				break
			}
		}
		if other == nil {
			t.Skip("directory has no second policy to distinguish the subentry branch")
		}

		if _, err := env.LDAP.UpdateUser(user.DN, ldap.UpdateUserRequest{
			PwdPolicySubentry: &other.DN,
		}); err != nil {
			t.Fatalf("set pwdPolicySubentry: %v", err)
		}

		policy := env.LDAP.EffectivePasswordPolicy(user.DN)
		if policy == nil {
			t.Fatal("no policy resolved for an entry carrying pwdPolicySubentry")
		}
		if policy.DN != other.DN {
			t.Errorf("resolved %q, want the entry's own subentry %q", policy.DN, other.DN)
		}
	})

	t.Run("unknowable answers are nil, not a guess", func(t *testing.T) {
		if policy := env.LDAP.EffectivePasswordPolicy("uid=does-not-exist," + env.LDAP.UserBaseDN()); policy != nil {
			t.Errorf("resolved %q for a missing entry, want nil", policy.DN)
		}
	})
}
