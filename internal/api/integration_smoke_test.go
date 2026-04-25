//go:build integration

package api

import (
	"net/http"
	"testing"
)

// TestIntegrationSmoke_Health verifies the test harness boots the full stack
// and the router answers a basic request. All other integration tests should
// rely on the same setupTestServer helper.
func TestIntegrationSmoke_Health(t *testing.T) {
	env := setupTestServer(t)

	resp, err := http.Get(env.Server.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
