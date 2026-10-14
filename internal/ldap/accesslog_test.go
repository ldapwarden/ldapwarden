package ldap

import (
	"reflect"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"
)

func raw(values ...string) [][]byte {
	out := make([][]byte, len(values))
	for i, v := range values {
		out[i] = []byte(v)
	}
	return out
}

// The reqMod values below are what slapd 2.6 logged for an ldapmodify on the
// bundled stack.
func TestParseReqMod(t *testing.T) {
	got := parseReqMod(raw(
		"description:= changed by ldapmodify",
		"userPassword:= {SSHA}50ZilytG+0pHobna8h76EHwRl+hB3wjo",
		"pwdChangedTime:= 20261014154018Z",
		"pwdHistory:+ 20261014154018Z#1.3.6.1.4.1.1466.115.121.1.40#38#{SSHA}SulLQBqsXskKQFnnfN9LIyAxbpibAnpA",
		"entryCSN:= 20261014154018.061916Z#000000#000#000000",
		"modifiersName:= cn=admin,dc=example,dc=org",
		"memberUid:+ alice",
		"memberUid:+ bob",
		"memberUid:- carol",
		"telephoneNumber:-",
		"jpegPhoto:= \xff\xd8\xff\x00",
	))
	want := []AttributeChange{
		{Attribute: "description", Op: "replace", Values: []string{"changed by ldapmodify"}},
		{Attribute: "userPassword", Op: "replace", Redacted: true},
		{Attribute: "pwdHistory", Op: "add", Redacted: true},
		{Attribute: "memberUid", Op: "add", Values: []string{"alice", "bob"}},
		{Attribute: "memberUid", Op: "delete", Values: []string{"carol"}},
		{Attribute: "telephoneNumber", Op: "delete"},
		{Attribute: "jpegPhoto", Op: "replace", Values: []string{"(binary, 4 bytes)"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseReqMod:\n got %+v\nwant %+v", got, want)
	}
}

func TestParseReqOld(t *testing.T) {
	got := parseReqOld(raw(
		"userPassword: {SSHA}SulLQBqsXskKQFnnfN9LIyAxbpibAnpA",
		"pwdChangedTime: 20261014154009Z",
		"memberUid: alice",
		"memberUid: bob",
	))
	want := []AttributeValues{
		{Attribute: "userPassword", Redacted: true},
		{Attribute: "memberUid", Values: []string{"alice", "bob"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseReqOld:\n got %+v\nwant %+v", got, want)
	}
}

func TestParseAccessLogEntryTime(t *testing.T) {
	entry := ldap.NewEntry("reqStart=20261014154018.000005Z,cn=accesslog", map[string][]string{
		"reqStart": {"20261014154018.000005Z"},
		"reqType":  {"modify"},
		"reqDN":    {"uid=jdoe,ou=People,dc=example,dc=org"},
	})
	change := parseAccessLogEntry(entry)
	want := time.Date(2026, 10, 14, 15, 40, 18, 5000, time.UTC)
	if !change.Time.Equal(want) {
		t.Errorf("time = %v, want %v", change.Time, want)
	}
}

func TestIsPolicyBookkeeping(t *testing.T) {
	mods := func(lines ...string) DirectoryChange {
		return DirectoryChange{Type: "modify", Changes: parseReqMod(raw(lines...))}
	}
	cases := []struct {
		name   string
		change DirectoryChange
		want   bool
	}{
		// What slapd logged for one failed bind on the bundled stack.
		{"failed bind", mods("pwdFailureTime:+ 20261014154031.113117Z", "entryCSN:= x", "modifiersName:= cn=admin,dc=example,dc=org"), true},
		{"lockout", mods("pwdFailureTime:+ 20261014154031Z", "pwdAccountLockedTime:+ 20261014154031Z"), true},
		{"lockout lifted at the next bind", mods("pwdFailureTime:-", "pwdAccountLockedTime:-"), true},
		{"grace login", mods("pwdGraceUseTime:+ 20261014154031Z"), true},
		// An administrator locking an account by hand writes the lock alone.
		{"lock written by hand", mods("pwdAccountLockedTime:= 000001010000Z"), false},
		{"anything else alongside", mods("pwdFailureTime:-", "description:= x"), false},
		{"not a modify", DirectoryChange{Type: "delete"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.change.IsPolicyBookkeeping(); got != tc.want {
				t.Errorf("IsPolicyBookkeeping = %v, want %v", got, tc.want)
			}
		})
	}
}
