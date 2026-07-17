package api

import (
	"fmt"
	"net/http"
	"testing"

	ldaplib "github.com/go-ldap/ldap/v3"
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
