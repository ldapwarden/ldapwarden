//go:build integration

package api

import (
	"net/http"
	"strings"
	"testing"
)

// TestIntegration_BodyLimit_RejectsOversizePayload exercises the
// maxBodyBytes middleware: a 1.5 MiB body on a route with the default
// 1 MiB cap (groups) must be refused with a 4xx, while a body up to a
// few MiB on /api/users (10 MiB cap) is accepted by the body reader and
// only rejected (or accepted) by the handler on its own merits.
func TestIntegration_BodyLimit_RejectsOversizePayload(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	// Forge a JSON document larger than the default 1 MiB cap by padding
	// the "description" field. The payload is syntactically valid JSON
	// so the body reader's 1 MiB limit, not json.Decode, fires first.
	pad := strings.Repeat("A", 1_500_000) // 1.5 MiB
	body := `{"cn":"x","gidNumber":99999,"description":"` + pad + `"}`

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/groups/",
		strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post oversized: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		t.Fatalf("oversized POST got %d, want a 4xx rejection", resp.StatusCode)
	}
	// http.MaxBytesReader surfaces an error that the chi/json layer turns
	// into a 400 Bad Request (or 413 if the response is written by the
	// reader itself). Either is acceptable as long as the request is
	// rejected.
	if resp.StatusCode < 400 || resp.StatusCode >= 500 {
		t.Errorf("oversized POST status=%d, want 4xx", resp.StatusCode)
	}
}
