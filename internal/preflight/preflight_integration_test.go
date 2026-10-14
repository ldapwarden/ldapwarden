//go:build integration

package preflight

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/config"
)

// bundledConfig points at the docker-compose stack, the way the server is
// configured there.
func bundledConfig(t *testing.T) *config.Config {
	t.Helper()
	if os.Getenv("LDAPWARDEN_TEST_INTEGRATION") != "1" {
		t.Skip("set LDAPWARDEN_TEST_INTEGRATION=1 (and start docker compose) to run integration tests")
	}
	cfg := validConfig()
	cfg.Database.URL = "postgres://ldapwarden:ldapwarden@localhost:5432/ldapwarden?sslmode=disable"
	cfg.LDAP = config.LDAPConfig{
		URL: "ldap://localhost:389", TLSMode: "none",
		BaseDN: "dc=example,dc=org", BindDN: "cn=admin,dc=example,dc=org", BindPass: "admin",
		UserOU: "ou=People", GroupOU: "ou=Groups", SudoersOU: "ou=sudoers",
		PpolicyOU: "ou=policies", NetgroupOU: "ou=netgroups",
		DefaultPolicyDN: "cn=default,ou=policies,dc=example,dc=org",
	}
	cfg.App.AdminGroup = "admins"
	return cfg
}

func TestConnectedBundledStack(t *testing.T) {
	cfg := bundledConfig(t)
	r := &Report{}
	Connected(context.Background(), r, cfg)
	if r.Failed() {
		t.Fatalf("the bundled stack fails the connected checks: %v", findings(r, Error))
	}
}

// TestConnectedNamesWhatTheDirectoryLacks: an admin group or a default policy
// that does not exist is what the server would only reveal later, as an admin
// page nobody can reach or lockouts resolved against nothing.
func TestConnectedNamesWhatTheDirectoryLacks(t *testing.T) {
	cfg := bundledConfig(t)
	cfg.App.AdminGroup = "no-such-group"
	cfg.LDAP.DefaultPolicyDN = "cn=absent,ou=policies,dc=example,dc=org"

	r := &Report{}
	Connected(context.Background(), r, cfg)
	errs := strings.Join(findings(r, Error), "\n")
	for _, want := range []string{`LDAPWARDEN_ADMIN_GROUP "no-such-group"`, "LDAP_DEFAULT_POLICY_DN"} {
		if !strings.Contains(errs, want) {
			t.Errorf("no error mentions %s; got:\n%s", want, errs)
		}
	}
}

func TestConnectedWrongBindPassword(t *testing.T) {
	cfg := bundledConfig(t)
	cfg.LDAP.BindPass = "wrong"
	r := &Report{}
	Connected(context.Background(), r, cfg)
	if !strings.Contains(strings.Join(findings(r, Error), "\n"), "cannot read the base entry") {
		t.Errorf("a failed bind is not reported: %v", r.Findings)
	}
}
