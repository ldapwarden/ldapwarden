//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	ldaplib "github.com/go-ldap/ldap/v3"

	"github.com/ldapwarden/ldapwarden/internal/config"
)

// requireAccessLog stops a test that needs the accesslog overlay when the
// running directory predates it. In CI the stack is built from ldap/, so a
// missing overlay there is a failure, not a reason to skip.
func requireAccessLog(t *testing.T, env *testEnv) {
	t.Helper()
	exists, err := env.LDAP.EntryExists("cn=accesslog")
	if err == nil && exists {
		return
	}
	msg := "the directory has no readable cn=accesslog: rebuild it from ldap/ " +
		"(docker compose build openldap && docker compose up -d --force-recreate -V openldap — this reseeds the directory)"
	if os.Getenv("CI") == "true" {
		t.Fatal(msg)
	}
	t.Skip(msg)
}

type directoryChangesBody struct {
	Status string `json:"status"`
	Data   []struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		DN         string `json:"dn"`
		Actor      string `json:"actor"`
		Origin     string `json:"origin"`
		AuditLogID string `json:"auditLogId"`
		Changes    []struct {
			Attribute string   `json:"attribute"`
			Op        string   `json:"op"`
			Values    []string `json:"values"`
			Redacted  bool     `json:"redacted"`
		} `json:"changes"`
	} `json:"data"`
	Counts map[string]int `json:"counts"`
}

func listDirectoryChanges(t *testing.T, env *testEnv, token, query string) directoryChangesBody {
	t.Helper()
	resp, body := doJSON(t, env, http.MethodGet, "/api/directory-changes?"+query, nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("directory changes: status=%d body=%s", resp.StatusCode, body)
	}
	var out directoryChangesBody
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, body)
	}
	return out
}

// TestIntegration_DirectoryChangesOrigin is the point of the feature: a write
// made around the app is reported as such, next to the app's own writes and the
// directory's bookkeeping, each told apart.
func TestIntegration_DirectoryChangesOrigin(t *testing.T) {
	env := setupTestServer(t)
	requireAccessLog(t, env)
	token := loginAs(t, env, "admin", "admin123").Token
	user := createTestUser(t, env, token) // through the app: audited

	// The account changes its own password straight against the directory, as
	// `ldappasswd` would: an identity that is not the app's, and no audit row.
	direct, err := ldaplib.DialURL(env.Config.LDAP.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = direct.Close() }()
	if err := direct.Bind(user.DN, "testpass"); err != nil {
		t.Fatalf("bind as the test user: %v", err)
	}
	if _, err := direct.PasswordModify(ldaplib.NewPasswordModifyRequest("", "", "changed-outside-1")); err != nil {
		t.Fatalf("self password change: %v", err)
	}
	// And a failed bind, which ppolicy records on the entry by itself.
	if err := direct.Bind(user.DN, "wrong-password"); err == nil {
		t.Fatal("bind with a wrong password succeeded")
	}

	all := listDirectoryChanges(t, env, token, "origin=all&search="+user.DN)
	var sawApp, sawExternal, sawPolicy bool
	for _, change := range all.Data {
		switch change.Origin {
		case originApp:
			if change.Type == "add" {
				sawApp = change.AuditLogID != ""
			}
		case originExternal:
			if !sameDN(change.Actor, user.DN) {
				t.Errorf("external change by %q: only the self-service password change was external", change.Actor)
			}
			sawExternal = true
			for _, c := range change.Changes {
				if strings.EqualFold(c.Attribute, "userPassword") && (!c.Redacted || len(c.Values) > 0) {
					t.Errorf("the password hash is shown: %+v", c)
				}
			}
		case originPolicy:
			sawPolicy = true
		}
	}
	if !sawApp {
		t.Error("the account's creation through the app is not attributed to the app")
	}
	if !sawExternal {
		t.Error("the password change made outside the app is not reported as external")
	}
	if !sawPolicy {
		t.Error("ppolicy's record of the failed bind is not reported as policy bookkeeping")
	}

	// The default view is the external writes alone.
	external := listDirectoryChanges(t, env, token, "search="+user.DN)
	for _, change := range external.Data {
		if change.Origin != originExternal {
			t.Errorf("default view shows a %s change", change.Origin)
		}
	}
	if len(external.Data) == 0 {
		t.Error("default view is empty")
	}
}

func TestIntegration_DirectoryChangesStatus(t *testing.T) {
	cases := []struct {
		name string
		base string
		want string
	}{
		{"not configured", "", "not-configured"},
		{"base the directory does not hold", "cn=no-such-log", "unreadable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := setupTestServerWith(t, func(cfg *config.Config) { cfg.LDAP.AccessLogBase = tc.base })
			token := loginAs(t, env, "admin", "admin123").Token
			if got := listDirectoryChanges(t, env, token, "").Status; got != tc.want {
				t.Errorf("status = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIntegration_DirectoryChangesAccess(t *testing.T) {
	env := setupTestServer(t)
	viewer := loginAs(t, env, "viewer", "viewer123").Token
	resp, body := doJSON(t, env, http.MethodGet, "/api/directory-changes", nil, viewer)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("read-only role: status=%d body=%s", resp.StatusCode, body)
	}

	admin := loginAs(t, env, "admin", "admin123").Token
	resp, _ = doJSON(t, env, http.MethodGet, "/api/directory-changes?origin=nobody", nil, admin)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("unknown origin: status=%d", resp.StatusCode)
	}
	from := time.Now().Format(time.DateOnly)
	to := time.Now().Add(-48 * time.Hour).Format(time.DateOnly)
	resp, _ = doJSON(t, env, http.MethodGet, "/api/directory-changes?from="+from+"&to="+to, nil, admin)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("from after to: status=%d", resp.StatusCode)
	}
}
