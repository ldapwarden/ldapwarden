//go:build integration

package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"testing"

	ldaplib "github.com/go-ldap/ldap/v3"
)

// ldifRecord is one entry read back from an export.
type ldifRecord struct {
	DN    string
	Attrs map[string][][]byte
	Order []string
}

// parseLDIF reads what ExportLDIF writes: folded lines, "name: value" and
// "name:: base64". Deliberately independent of the writer, so the two cannot
// share a mistake.
func parseLDIF(t *testing.T, data []byte) (records []ldifRecord, complete bool) {
	t.Helper()
	text := strings.ReplaceAll(string(data), "\n ", "")
	var current *ldifRecord
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "# end of export"):
			complete = true
			continue
		case line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "version:"):
			if line == "" && current != nil {
				records = append(records, *current)
				current = nil
			}
			continue
		}
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			t.Fatalf("not an LDIF line: %q", line)
		}
		var value []byte
		if strings.HasPrefix(rest, ":") {
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(rest[1:]))
			if err != nil {
				t.Fatalf("bad base64 in %q: %v", line, err)
			}
			value = decoded
		} else {
			value = []byte(strings.TrimPrefix(rest, " "))
		}
		if name == "dn" {
			current = &ldifRecord{DN: string(value), Attrs: map[string][][]byte{}}
			continue
		}
		if _, seen := current.Attrs[name]; !seen {
			current.Order = append(current.Order, name)
		}
		current.Attrs[name] = append(current.Attrs[name], value)
	}
	return records, complete
}

func exportPath(base, scope string) string {
	q := url.Values{}
	if base != "" {
		q.Set("base", base)
	}
	if scope != "" {
		q.Set("scope", scope)
	}
	return "/api/admin/export?" + q.Encode()
}

// TestIntegration_ExportRoundTrip is what a backup is for: the entry comes back
// from the file as it was. That covers a binary photo, which only survives
// base64, and the account's lock — pwdAccountLockedTime is operational, so an
// export of "*" alone restores every locked account unlocked.
func TestIntegration_ExportRoundTrip(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token
	user := createTestUser(t, env, token)

	photo := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, '\n', ' ', 0xff, 0xd9}
	modify := ldaplib.NewModifyRequest(user.DN, nil)
	modify.Replace("jpegPhoto", []string{string(photo)})
	if err := env.LDAP.Modify(modify); err != nil {
		t.Fatalf("set photo: %v", err)
	}
	if resp, body := doJSON(t, env, http.MethodPost, userPath(user.DN)+"/lock", nil, token); resp.StatusCode != http.StatusOK {
		t.Fatalf("lock: status=%d body=%s", resp.StatusCode, body)
	}

	resp, body := doJSON(t, env, http.MethodGet, exportPath(user.DN, "base"), nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export: status=%d body=%s", resp.StatusCode, body)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") || !strings.Contains(cd, ".ldif") {
		t.Errorf("Content-Disposition = %q", cd)
	}
	records, complete := parseLDIF(t, body)
	if !complete {
		t.Fatalf("export has no end marker:\n%s", body)
	}
	if len(records) != 1 {
		t.Fatalf("exported %d entries for a base-scope export, want 1", len(records))
	}
	record := records[0]
	if got := record.Attrs["jpegPhoto"]; len(got) != 1 || !bytes.Equal(got[0], photo) {
		t.Fatalf("jpegPhoto did not survive the export: %v", got)
	}
	if got := record.Attrs["pwdAccountLockedTime"]; len(got) != 1 || string(got[0]) != "000001010000Z" {
		t.Fatalf("the lock is missing from the export: %v", got)
	}

	// Restore it the way ldapadd would: every attribute of the record, in one add.
	if err := env.LDAP.Delete(user.DN); err != nil {
		t.Fatalf("delete before restore: %v", err)
	}
	add := ldaplib.NewAddRequest(record.DN, nil)
	for _, name := range record.Order {
		values := make([]string, len(record.Attrs[name]))
		for i, v := range record.Attrs[name] {
			values[i] = string(v)
		}
		add.Attribute(name, values)
	}
	if err := env.LDAP.Add(add); err != nil {
		t.Fatalf("the exported entry cannot be added back: %v", err)
	}

	restored, err := env.LDAP.GetEntry(user.DN, []string{"jpegPhoto", "pwdAccountLockedTime"})
	if err != nil {
		t.Fatalf("read restored entry: %v", err)
	}
	if !bytes.Equal(restored.GetRawAttributeValue("jpegPhoto"), photo) {
		t.Error("restored photo differs")
	}
	if got := restored.GetAttributeValue("pwdAccountLockedTime"); got != "000001010000Z" {
		t.Errorf("restored entry is not locked: pwdAccountLockedTime=%q", got)
	}

	var audited int
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_logs WHERE action = 'directory.export' AND resource_dn = $1`, user.DN,
	).Scan(&audited); err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if audited != 1 {
		t.Errorf("audit rows for the export = %d, want 1", audited)
	}
}

// TestIntegration_ExportWholeDirectory checks the default export: the whole
// base, complete, and in an order ldapadd can replay — no entry before its
// parent.
func TestIntegration_ExportWholeDirectory(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	resp, body := doJSON(t, env, http.MethodGet, "/api/admin/export", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export: status=%d body=%s", resp.StatusCode, body)
	}
	records, complete := parseLDIF(t, body)
	if !complete {
		t.Fatal("export has no end marker")
	}
	if len(records) < 10 || !strings.EqualFold(records[0].DN, env.Config.LDAP.BaseDN) {
		t.Fatalf("expected the base entry first and the seed under it; got %d entries starting with %q",
			len(records), records[0].DN)
	}

	seen := map[string]bool{}
	for _, record := range records {
		dn, err := ldaplib.ParseDN(record.DN)
		if err != nil {
			t.Fatalf("exported DN %q does not parse: %v", record.DN, err)
		}
		key := strings.ToLower(record.DN)
		if len(seen) > 0 {
			parent := &ldaplib.DN{RDNs: dn.RDNs[1:]}
			found := false
			for prior := range seen {
				if priorDN, _ := ldaplib.ParseDN(prior); priorDN != nil && priorDN.EqualFold(parent) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s comes before its parent", record.DN)
			}
		}
		seen[key] = true
	}
}

func TestIntegration_ExportRefusals(t *testing.T) {
	env := setupTestServer(t)
	admin := loginAs(t, env, "admin", "admin123").Token
	viewer := loginAs(t, env, "viewer", "viewer123").Token

	cases := []struct {
		name  string
		path  string
		token string
		want  int
	}{
		{"read-only role", "/api/admin/export", viewer, http.StatusForbidden},
		{"unknown scope", exportPath("", "children"), admin, http.StatusBadRequest},
		{"base that is not a DN", exportPath("not a dn", ""), admin, http.StatusBadRequest},
		{"base the directory does not hold", exportPath("ou=absent,dc=example,dc=org", ""), admin, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := doJSON(t, env, http.MethodGet, tc.path, nil, tc.token)
			if resp.StatusCode != tc.want {
				t.Errorf("status = %d, want %d (body=%s)", resp.StatusCode, tc.want, body)
			}
		})
	}
}
