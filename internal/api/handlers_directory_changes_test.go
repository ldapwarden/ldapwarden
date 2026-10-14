package api

import (
	"testing"
	"time"

	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/config"
	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

func TestClassifyChange(t *testing.T) {
	const bindDN = "cn=admin,dc=example,dc=org"
	s := &Server{config: &config.Config{LDAP: config.LDAPConfig{BindDN: bindDN}}}
	at := time.Date(2026, 10, 14, 15, 0, 0, 0, time.UTC)
	writes := []audit.DirectoryWrite{
		{ID: "rename", ActorDN: "uid=admin,ou=People,dc=example,dc=org", CreatedAt: at.Add(-10 * time.Second)},
		{ID: "self-service", ActorDN: "uid=jdoe,ou=People,dc=example,dc=org", CreatedAt: at.Add(time.Hour)},
	}
	modify := func(actor string, when time.Time) ldap.DirectoryChange {
		return ldap.DirectoryChange{
			Type: "modify", Actor: actor, Time: when, DN: "cn=dev,ou=Groups,dc=example,dc=org",
			Changes: []ldap.AttributeChange{{Attribute: "memberUid", Op: "add", Values: []string{"x"}}},
		}
	}

	cases := []struct {
		name   string
		change ldap.DirectoryChange
		origin string
		audit  string
	}{
		// A rename's memberUid rewrite, seconds after the audit row: the app.
		{"app identity during an audited request", modify(bindDN, at), originApp, "rename"},
		{"app identity spelled differently", modify("CN=Admin, DC=example,DC=org", at), originApp, "rename"},
		// The same identity while the app recorded nothing: someone else holding
		// the app's credentials, or the rootdn shared with an ldapmodify.
		{"app identity while the app was idle", modify(bindDN, at.Add(10*time.Minute)), originExternal, ""},
		{"another identity during an audited request", modify("cn=script,dc=example,dc=org", at), originExternal, ""},
		{"the user a self-service change was for", modify("uid=jdoe,ou=People,dc=example,dc=org", at.Add(time.Hour)), originApp, "self-service"},
		{"no identity", modify("", at), originExternal, ""},
		{"ppolicy recording a failed bind", ldap.DirectoryChange{
			Type: "modify", Actor: bindDN, Time: at,
			Changes: []ldap.AttributeChange{{Attribute: "pwdFailureTime", Op: "add", Values: []string{"x"}}},
		}, originPolicy, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := s.classifyChange(tc.change, writes)
			if got.Origin != tc.origin || got.AuditLogID != tc.audit {
				t.Errorf("origin = %q (audit %q), want %q (audit %q)", got.Origin, got.AuditLogID, tc.origin, tc.audit)
			}
		})
	}
}
