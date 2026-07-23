//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

type importResultBody struct {
	Created int `json:"created"`
	Failed  int `json:"failed"`
	Results []struct {
		Index  int    `json:"index"`
		Key    string `json:"key"`
		Status string `json:"status"`
		Error  string `json:"error"`
	} `json:"results"`
}

// TestIntegration_Users_Import imports a batch with two valid rows and one
// invalid row (a uid the RDN validator rejects). The valid rows must be
// created, the invalid row reported as an error and skipped, and the whole
// request must still succeed (no all-or-nothing).
func TestIntegration_Users_Import(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	suffix := uniqueSuffix(t)
	n := testIDNumber(suffix)
	good1 := "import-a-" + suffix
	good2 := "import-b-" + suffix

	rows := []map[string]any{
		{"uid": good1, "givenName": "Imp", "sn": "One", "uidNumber": n, "gidNumber": n},
		{"uid": "bad uid " + suffix, "givenName": "Imp", "sn": "Bad", "uidNumber": n + 1, "gidNumber": n + 1},
		{"uid": good2, "givenName": "Imp", "sn": "Two", "uidNumber": n + 2, "gidNumber": n + 2},
	}
	t.Cleanup(func() {
		for _, uid := range []string{good1, good2} {
			_, _ = doJSON(t, env, http.MethodDelete,
				userPath("uid="+uid+",ou=People,dc=example,dc=org"), nil, token)
		}
	})

	resp, body := doJSON(t, env, http.MethodPost, "/api/users/import",
		map[string]any{"rows": rows}, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("import: status=%d body=%s", resp.StatusCode, body)
	}

	var result importResultBody
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, body)
	}
	if result.Created != 2 || result.Failed != 1 {
		t.Fatalf("created=%d failed=%d, want 2/1 (body=%s)", result.Created, result.Failed, body)
	}
	if len(result.Results) != 3 {
		t.Fatalf("results len=%d, want 3", len(result.Results))
	}
	if result.Results[1].Status != "error" || result.Results[1].Error == "" {
		t.Errorf("row 1 (bad uid) should be an error with a message, got %+v", result.Results[1])
	}
	if result.Results[0].Status != "created" || result.Results[2].Status != "created" {
		t.Errorf("valid rows should be created, got %+v and %+v", result.Results[0], result.Results[2])
	}

	// The first valid user must now exist.
	resp, body = doJSON(t, env, http.MethodGet,
		userPath("uid="+good1+",ou=People,dc=example,dc=org"), nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("get imported user: status=%d body=%s", resp.StatusCode, body)
	}
}

// TestIntegration_Users_Import_RejectsTooManyRows guards the row cap.
func TestIntegration_Users_Import_RejectsTooManyRows(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	rows := make([]map[string]any, maxImportRows+1)
	for i := range rows {
		rows[i] = map[string]any{"uid": "x", "givenName": "x", "sn": "x", "uidNumber": 10000, "gidNumber": 10000}
	}
	resp, body := doJSON(t, env, http.MethodPost, "/api/users/import",
		map[string]any{"rows": rows}, token)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversize import: status=%d body=%s, want 400", resp.StatusCode, body)
	}
}

// TestIntegration_Groups_Import imports one valid and one invalid group.
func TestIntegration_Groups_Import(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	suffix := uniqueSuffix(t)
	n := testIDNumber(suffix)
	good := "impgrp-" + suffix

	rows := []map[string]any{
		{"cn": good, "gidNumber": n, "description": "imported"},
		{"cn": "bad cn " + suffix, "gidNumber": n + 1},
	}
	t.Cleanup(func() {
		_, _ = doJSON(t, env, http.MethodDelete,
			groupPath("cn="+good+",ou=Groups,dc=example,dc=org"), nil, token)
	})

	resp, body := doJSON(t, env, http.MethodPost, "/api/groups/import",
		map[string]any{"rows": rows}, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("import: status=%d body=%s", resp.StatusCode, body)
	}
	var result importResultBody
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, body)
	}
	if result.Created != 1 || result.Failed != 1 {
		t.Fatalf("created=%d failed=%d, want 1/1 (body=%s)", result.Created, result.Failed, body)
	}
}
