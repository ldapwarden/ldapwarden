//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func decodeHealth(t *testing.T, body []byte) healthResponse {
	t.Helper()
	var out healthResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal health response: %v (body=%s)", err, body)
	}
	return out
}

// TestIntegration_Probes_NeedNoSession pins that both probes answer without
// credentials. A probe an orchestrator has to authenticate for is a probe nobody
// can use.
func TestIntegration_Probes_NeedNoSession(t *testing.T) {
	env := setupTestServer(t)

	for _, path := range []string{"/healthz", "/readyz", "/health"} {
		resp, body := doJSON(t, env, http.MethodGet, path, nil, "")
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s without a session: status=%d body=%s, want 200", path, resp.StatusCode, body)
		}
	}
}

// TestIntegration_Liveness_ReportsOnlyTheProcess is the distinction the split
// exists for: liveness must not consult a dependency, because a failing liveness
// probe gets the container restarted and restarting fixes nothing about an
// unreachable directory.
func TestIntegration_Liveness_ReportsOnlyTheProcess(t *testing.T) {
	env := setupTestServer(t)

	resp, body := doJSON(t, env, http.MethodGet, "/healthz", nil, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("liveness: status=%d body=%s", resp.StatusCode, body)
	}
	health := decodeHealth(t, body)
	if health.Status != "ok" {
		t.Errorf("status = %q, want %q", health.Status, "ok")
	}
	if len(health.Checks) != 0 {
		t.Errorf("liveness reported dependency checks (%v); that belongs in /readyz", health.Checks)
	}
}

// TestIntegration_Readiness_ChecksEveryDependency pins the three names, because
// the point of the endpoint is telling an operator *which* dependency is down.
// Dependencies being down is not simulated here — the suite needs them all — so
// this covers the shape and the healthy verdict.
func TestIntegration_Readiness_ChecksEveryDependency(t *testing.T) {
	env := setupTestServer(t)

	resp, body := doJSON(t, env, http.MethodGet, "/readyz", nil, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("readiness against a working stack: status=%d body=%s", resp.StatusCode, body)
	}
	health := decodeHealth(t, body)
	if health.Status != "ready" {
		t.Errorf("status = %q, want %q", health.Status, "ready")
	}
	for _, name := range []string{"database", "sessions", "directory"} {
		got, ok := health.Checks[name]
		if !ok {
			t.Errorf("readiness does not report %q: %v", name, health.Checks)
			continue
		}
		if got != "ok" {
			t.Errorf("%s = %q, want ok", name, got)
		}
	}
}

// TestIntegration_Health_AliasMatchesHealthz keeps the deprecated path honest for
// as long as it exists: something still pointing at it must get the same answer,
// not a redirect or a stale body.
func TestIntegration_Health_AliasMatchesHealthz(t *testing.T) {
	env := setupTestServer(t)

	_, healthzBody := doJSON(t, env, http.MethodGet, "/healthz", nil, "")
	_, aliasBody := doJSON(t, env, http.MethodGet, "/health", nil, "")
	if string(healthzBody) != string(aliasBody) {
		t.Errorf("/health returned %s, /healthz returned %s", aliasBody, healthzBody)
	}
}
