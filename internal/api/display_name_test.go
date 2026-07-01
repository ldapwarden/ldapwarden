package api

import (
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

func TestUserDisplayName(t *testing.T) {
	cases := []struct {
		name string
		user ldap.User
		want string
	}{
		{"displayName qualified with uid", ldap.User{UID: "rbesson", DisplayName: "Romain Besson"}, "Romain Besson (rbesson)"},
		{"givenName+sn fallback", ldap.User{UID: "adefina", GivenName: "Anna", SN: "De Fina"}, "Anna De Fina (adefina)"},
		{"cn fallback", ldap.User{UID: "jhawken", CN: "James Hawken"}, "James Hawken (jhawken)"},
		{"no friendly name uses bare uid", ldap.User{UID: "aduffez"}, "aduffez"},
		{"friendly equal to uid is not duplicated", ldap.User{UID: "aduffez", DisplayName: "aduffez"}, "aduffez"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := userDisplayName(&tc.user); got != tc.want {
				t.Errorf("userDisplayName = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCreateUserDisplayName(t *testing.T) {
	got := createUserDisplayName(ldap.CreateUserRequest{UID: "rbesson", DisplayName: "Romain Besson"})
	if want := "Romain Besson (rbesson)"; got != want {
		t.Errorf("createUserDisplayName = %q, want %q", got, want)
	}
	if got := createUserDisplayName(ldap.CreateUserRequest{UID: "aduffez"}); got != "aduffez" {
		t.Errorf("createUserDisplayName = %q, want %q", got, "aduffez")
	}
}

func TestGroupDisplayName(t *testing.T) {
	cases := []struct {
		name  string
		group ldap.Group
		want  string
	}{
		{"description qualified with cn", ldap.Group{CN: "engineers", Description: "Engineering team"}, "Engineering team (engineers)"},
		{"samba displayName fallback", ldap.Group{CN: "engineers", DisplayName: "Engineers"}, "Engineers (engineers)"},
		{"no friendly name uses bare cn", ldap.Group{CN: "engineers"}, "engineers"},
		{"description preferred over displayName", ldap.Group{CN: "engineers", Description: "Engineering team", DisplayName: "Engineers"}, "Engineering team (engineers)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := groupDisplayName(&tc.group); got != tc.want {
				t.Errorf("groupDisplayName = %q, want %q", got, tc.want)
			}
		})
	}
}
