//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestIntegration_Audit_ListWithIP guards the INET-scan regression: every real
// request records a non-null ip_address (INET) on its audit row, and pgx
// cannot scan INET into *string. Listing audit logs therefore 500'd in
// production (it only ever worked when all visible rows had a null IP) until
// the query was changed to host(ip_address). Logging in is itself an audited
// action whose row carries the caller IP, so we log in and then list.
func TestIntegration_Audit_ListWithIP(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	resp, body := doJSON(t, env, http.MethodGet, "/api/audit-logs", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/audit-logs: status=%d body=%s, want 200", resp.StatusCode, body)
	}

	var parsed struct {
		Data []struct {
			Action    string `json:"action"`
			IPAddress string `json:"ipAddress"`
		} `json:"data"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, body)
	}
	if parsed.Total == 0 || len(parsed.Data) == 0 {
		t.Fatalf("expected at least the login audit row, got total=%d len=%d", parsed.Total, len(parsed.Data))
	}

	// At least one row must carry a non-empty IP — the value that broke the scan.
	hasIP := false
	for _, e := range parsed.Data {
		if e.IPAddress != "" {
			hasIP = true
			break
		}
	}
	if !hasIP {
		t.Errorf("no audit row carried an ip_address; cannot assert the INET path is exercised")
	}
}
