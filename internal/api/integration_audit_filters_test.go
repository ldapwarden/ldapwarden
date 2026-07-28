//go:build integration

package api

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestIntegration_AuditLogs_SearchAndDateFilters checks the filters the audit
// page relies on: a substring match that spans actor and resource, and day
// bounds that include the whole end day.
func TestIntegration_AuditLogs_SearchAndDateFilters(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	// Creating a user writes a user.create entry naming it.
	user := createTestUser(t, env, token)

	byResource := listAuditLogs(t, env, token, "?search="+url.QueryEscape(user.UID))
	if byResource.Total < 1 {
		t.Fatalf("search on the resource uid found nothing (total=%d)", byResource.Total)
	}
	for _, entry := range byResource.Data {
		if !strings.Contains(entry.ResourceDN, user.UID) && !strings.Contains(entry.ActorUID, user.UID) {
			t.Errorf("entry matched neither actor nor resource: actor=%q resource=%q", entry.ActorUID, entry.ResourceDN)
		}
	}

	byActor := listAuditLogs(t, env, token, "?actorUid=admin&limit=5")
	if byActor.Total == 0 {
		t.Fatal("actorUid=admin matched nothing")
	}
	for _, entry := range byActor.Data {
		if entry.ActorUID != "admin" {
			t.Errorf("actorUid filter returned an entry from %q", entry.ActorUID)
		}
	}

	// The entry was written today, so today's bounds must include it — this is
	// the regression guard for `to` being treated as midnight.
	today := time.Now().Format(time.DateOnly)
	todayOnly := listAuditLogs(t, env, token, "?from="+today+"&to="+today+"&search="+url.QueryEscape(user.UID))
	if todayOnly.Total < 1 {
		t.Errorf("from=to=today excluded an entry written today (total=%d)", todayOnly.Total)
	}

	// A window that ends before the entry existed must exclude it.
	past := listAuditLogs(t, env, token, "?to=2000-01-01&search="+url.QueryEscape(user.UID))
	if past.Total != 0 {
		t.Errorf("to=2000-01-01 returned %d entries", past.Total)
	}
}

// TestIntegration_AuditLogs_Actors checks the actor list used by the filter.
func TestIntegration_AuditLogs_Actors(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	resp, body := doJSON(t, env, http.MethodGet, "/api/audit-logs/actors", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list actors: status=%d body=%s", resp.StatusCode, body)
	}
	var out struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// The login above was itself audited, so admin has to be in there.
	found := false
	for _, uid := range out.Data {
		if uid == "admin" {
			found = true
		}
	}
	if !found {
		t.Errorf("admin missing from the actor list: %v", out.Data)
	}
}

// TestIntegration_AuditLogs_ExportCSV checks the export is well-formed CSV,
// honours the filters, and renders the structured diff rather than raw JSON.
func TestIntegration_AuditLogs_ExportCSV(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	user := createTestUser(t, env, token)
	// An update produces a details.changes diff to render.
	if resp, body := doJSON(t, env, http.MethodPut, userPath(user.DN),
		map[string]any{"title": "Auditor"}, token); resp.StatusCode != http.StatusOK {
		t.Fatalf("update user: status=%d body=%s", resp.StatusCode, body)
	}

	query := "?search=" + url.QueryEscape(user.UID)
	resp, body := doJSON(t, env, http.MethodGet, "/api/audit-logs/export"+query, nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export: status=%d body=%s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type = %q, want text/csv", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Errorf("Content-Disposition = %q, want an attachment", cd)
	}

	records, err := csv.NewReader(strings.NewReader(string(body))).ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v (body=%s)", err, body)
	}
	if len(records) < 2 {
		t.Fatalf("export has no data rows: %v", records)
	}

	header := records[0]
	wantHeader := []string{
		"timestamp", "actor_uid", "actor_dn", "action",
		"resource_type", "resource_dn", "changes", "details", "ip_address", "user_agent",
	}
	if strings.Join(header, ",") != strings.Join(wantHeader, ",") {
		t.Errorf("header = %v, want %v", header, wantHeader)
	}

	// Oldest first: the create precedes the update.
	var actions []string
	changesColumn := -1
	for i, name := range header {
		if name == "changes" {
			changesColumn = i
		}
	}
	sawDiff := false
	for _, row := range records[1:] {
		actions = append(actions, row[3])
		if strings.Contains(row[changesColumn], "→") {
			sawDiff = true
		}
	}
	if len(actions) < 2 || actions[0] != "user.create" {
		t.Errorf("export not oldest-first: actions=%v", actions)
	}
	if !sawDiff {
		t.Errorf("no rendered diff in the changes column: %v", records[1:])
	}

	// A timestamp must parse as RFC 3339 so a spreadsheet can read it.
	if _, err := time.Parse(time.RFC3339, records[1][0]); err != nil {
		t.Errorf("timestamp %q is not RFC 3339: %v", records[1][0], err)
	}
}

// TestIntegration_AuditLogs_ExportRequiresPermission keeps the export behind the
// same permission as the listing — it returns the same data in bulk.
func TestIntegration_AuditLogs_ExportRequiresPermission(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "viewer", "viewer123").Token

	for _, path := range []string{"/api/audit-logs/export", "/api/audit-logs/actors"} {
		resp, body := doJSON(t, env, http.MethodGet, path, nil, token)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s as viewer: status=%d body=%s, want 403", path, resp.StatusCode, body)
		}
	}
}

type auditListResponse struct {
	Data []struct {
		ActorUID   string `json:"actorUid"`
		Action     string `json:"action"`
		ResourceDN string `json:"resourceDn"`
	} `json:"data"`
	Total int `json:"total"`
}

func listAuditLogs(t *testing.T, env *testEnv, token, query string) auditListResponse {
	t.Helper()
	resp, body := doJSON(t, env, http.MethodGet, "/api/audit-logs/"+query, nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list audit logs %q: status=%d body=%s", query, resp.StatusCode, body)
	}
	var out auditListResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal audit list: %v (body=%s)", err, body)
	}
	return out
}
