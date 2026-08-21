package api

import (
	"fmt"
	"net/http"
	"testing"

	ldaplib "github.com/go-ldap/ldap/v3"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

func TestLDAPErrorResponse(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantOK     bool
		wantStatus int
	}{
		{
			name:       "object class violation (missing schema)",
			err:        &ldaplib.Error{ResultCode: ldaplib.LDAPResultObjectClassViolation},
			wantOK:     true,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "invalid attribute syntax, wrapped",
			err:        fmt.Errorf("create sudo role: %w", &ldaplib.Error{ResultCode: ldaplib.LDAPResultInvalidAttributeSyntax}),
			wantOK:     true,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "entry already exists -> 409",
			err:        &ldaplib.Error{ResultCode: ldaplib.LDAPResultEntryAlreadyExists},
			wantOK:     true,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "insufficient access -> 403",
			err:        &ldaplib.Error{ResultCode: ldaplib.LDAPResultInsufficientAccessRights},
			wantOK:     true,
			wantStatus: http.StatusForbidden,
		},
		{
			name:   "non-ldap error falls through to 500",
			err:    fmt.Errorf("some pgx failure"),
			wantOK: false,
		},
		{
			name:   "unmapped ldap code falls through to 500",
			err:    &ldaplib.Error{ResultCode: ldaplib.LDAPResultBusy},
			wantOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg, status, ok := ldapErrorResponse(tc.err)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if status != tc.wantStatus {
				t.Errorf("status = %d, want %d", status, tc.wantStatus)
			}
			if msg == "" {
				t.Error("expected a non-empty client message")
			}
		})
	}
}

// TestPasswordRules covers the sentence writePasswordError appends to a policy
// rejection. The generic constraintViolation message ("a uniqueness or value
// rule") named nothing an operator could act on, so this is the part that has to
// stay accurate: an empty string whenever the policy is unknown or sets none of
// these, and never a rule the policy does not actually impose.
func TestPasswordRules(t *testing.T) {
	cases := []struct {
		name   string
		policy *ldap.PasswordPolicy
		want   string
	}{
		{
			name:   "no policy to read",
			policy: nil,
			want:   "",
		},
		{
			name:   "policy sets none of the actionable rules",
			policy: &ldap.PasswordPolicy{CN: "loose"},
			want:   "",
		},
		{
			name:   "minimum length only",
			policy: &ldap.PasswordPolicy{CN: "default", PwdMinLength: 8},
			want:   "Policy default: at least 8 characters.",
		},
		{
			name: "length and history",
			policy: &ldap.PasswordPolicy{
				CN: "default", PwdMinLength: 8, PwdInHistory: 5,
			},
			want: "Policy default: at least 8 characters and not one of the last 5 passwords used.",
		},
		{
			name: "all three, as the bundled default policy sets them",
			policy: &ldap.PasswordPolicy{
				CN: "strict", PwdMinLength: 12, PwdInHistory: 10, PwdCheckQuality: 2,
			},
			want: "Policy strict: at least 12 characters, not one of the last 10 passwords used and complex enough for the directory's quality check.",
		},
		{
			name:   "quality check alone reads as a sentence, not a dangling conjunction",
			policy: &ldap.PasswordPolicy{CN: "quality", PwdCheckQuality: 1},
			want:   "Policy quality: complex enough for the directory's quality check.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := passwordRules(tc.policy); got != tc.want {
				t.Errorf("passwordRules() =\n  %q\nwant\n  %q", got, tc.want)
			}
		})
	}
}
