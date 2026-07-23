package api

import (
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

func TestValidateImportUser(t *testing.T) {
	valid := ldap.CreateUserRequest{UID: "alice", GivenName: "Alice", SN: "Smith", UIDNumber: 2000, GIDNumber: 2000}
	if err := validateImportUser(valid, 1000, 1000); err != nil {
		t.Errorf("valid row rejected: %v", err)
	}

	cases := map[string]ldap.CreateUserRequest{
		"missing sn/givenName": {UID: "alice", UIDNumber: 2000, GIDNumber: 2000},
		"bad uid":              {UID: "alice smith", GivenName: "Alice", SN: "Smith", UIDNumber: 2000, GIDNumber: 2000},
		"uid below floor":      {UID: "alice", GivenName: "Alice", SN: "Smith", UIDNumber: 10, GIDNumber: 2000},
		"bad group cn":         {UID: "alice", GivenName: "Alice", SN: "Smith", UIDNumber: 2000, GIDNumber: 2000, Groups: []string{"ok", "bad cn"}},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validateImportUser(req, 1000, 1000); err == nil {
				t.Errorf("expected rejection for %s", name)
			}
		})
	}
}

func TestValidateImportGroup(t *testing.T) {
	if err := validateImportGroup(ldap.CreateGroupRequest{CN: "eng", GIDNumber: 2000}, 1000); err != nil {
		t.Errorf("valid group rejected: %v", err)
	}
	if err := validateImportGroup(ldap.CreateGroupRequest{CN: "bad cn", GIDNumber: 2000}, 1000); err == nil {
		t.Errorf("expected rejection for a cn with a space")
	}
	if err := validateImportGroup(ldap.CreateGroupRequest{CN: "eng", GIDNumber: 10}, 1000); err == nil {
		t.Errorf("expected rejection for a gidNumber below the floor")
	}
}
