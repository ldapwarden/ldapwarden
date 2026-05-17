//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestIntegration_ServerError_HidesInternalDiagnostics fires a request
// that lands on the writeServerError path (an LDAP "no such object"
// inside handleUpdateUser) and asserts the JSON response no longer
// echoes the underlying LDAP/library error verbatim. Instead it carries
// a generic message + a request ID for support correlation.
func TestIntegration_ServerError_HidesInternalDiagnostics(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	// Well-formed DN, but no entry exists at that DN. resolveDN and the
	// audit insert both succeed, so the failure lands on the LDAP modify
	// step — the codepath writeServerError covers.
	bogusDN := "uid=ghost-" + uniqueSuffix(t) + ",ou=People,dc=example,dc=org"
	resp, body := doJSON(t, env, http.MethodPut,
		"/api/users/"+url.PathEscape(bogusDN),
		map[string]any{"sn": "X"}, token)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("PUT ghost user: status=%d body=%s, want 500", resp.StatusCode, body)
	}

	var parsed map[string]string
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal body: %v body=%s", err, body)
	}
	msg := parsed["error"]

	if !strings.Contains(msg, "internal server error") {
		t.Errorf("body does not carry generic message: %q", msg)
	}
	// chi.middleware.RequestID is mounted globally, so every response
	// should carry a non-empty correlation handle.
	if !strings.Contains(msg, "requestId=") {
		t.Errorf("body has no requestId for correlation: %q", msg)
	}

	// The original LDAP / library diagnostics must not leak.
	forbidden := []string{
		"LDAP Result Code",   // go-ldap result wrapping
		"no such object",     // LDAP server reason phrase
		"failed to update",   // the old hand-rolled prefix we replaced
		"go-ldap",            // library marker
	}
	for _, needle := range forbidden {
		if strings.Contains(msg, needle) {
			t.Errorf("response body leaks internal diagnostic %q: %q", needle, msg)
		}
	}
}
