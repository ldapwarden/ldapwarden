//go:build integration

package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// TestIntegration_Audit_RefusesMutationWhenLogFails proves the audit-first
// contract: if the audit insert fails, the handler must respond 500 and
// must NOT touch LDAP. We break the audit_logs table mid-test by renaming
// it, then verify a mutation 500s and that the directory state is unchanged.
func TestIntegration_Audit_RefusesMutationWhenLogFails(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	user := createTestUser(t, env, token)
	originalSN := user.SN

	// Break the audit table so every audit_logs INSERT fails. Restore
	// before any subtest cleanup or other tests run so the suite stays
	// stable.
	ctx := context.Background()
	if _, err := env.Pool.Exec(ctx, "ALTER TABLE audit_logs RENAME TO audit_logs_offline"); err != nil {
		t.Fatalf("rename audit_logs: %v", err)
	}
	defer func() {
		if _, err := env.Pool.Exec(ctx, "ALTER TABLE audit_logs_offline RENAME TO audit_logs"); err != nil {
			t.Errorf("restore audit_logs: %v", err)
		}
	}()

	// Attempt to update the user's SN. The pre-fix behaviour silently
	// swallowed the audit error and returned 200 with the LDAP change
	// applied — the regression we are guarding against.
	resp, body := doJSON(t, env, http.MethodPut, userPath(user.DN),
		map[string]any{"sn": "ShouldNotPersist"}, token)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("PUT user with broken audit: status=%d body=%s, want 500", resp.StatusCode, body)
	}

	// Directly read the LDAP entry through the test's client to confirm
	// the directory was never mutated.
	got, err := env.LDAP.GetUser(user.DN)
	if err != nil {
		t.Fatalf("GetUser after refused mutation: %v", err)
	}
	if got.SN != originalSN {
		t.Errorf("LDAP SN was changed to %q despite audit failure; want unchanged %q", got.SN, originalSN)
	}

	// Sanity: still no audit_logs rows survived (audit_logs_offline holds
	// whatever rows pre-existed; nothing should have been written to a
	// recreated audit_logs).
	var hasTable bool
	_ = env.Pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'audit_logs')").
		Scan(&hasTable)
	if hasTable {
		t.Errorf("audit_logs table was unexpectedly recreated during the failed request")
	}

	// Ensure the entry struct field name we assert on does exist on the
	// shared User type (defensive: this test breaks loudly if the struct
	// is renamed).
	var _ ldap.User = *got
}
