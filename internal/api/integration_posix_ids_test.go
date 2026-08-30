//go:build integration

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// requireSambaSchema skips unless the directory can hold sambaSamAccount.
func requireSambaSchema(t *testing.T, env *testEnv, token string) {
	t.Helper()

	probe := createTestUser(t, env, token)
	t.Cleanup(func() { _, _ = doJSON(t, env, http.MethodDelete, userPath(probe.DN), nil, token) })

	sid := "S-1-5-21-0-0-0-1"
	if _, err := env.LDAP.SetSambaUserAttributes(probe.DN,
		ldap.UpdateSambaUserRequest{SambaSID: &sid}); err != nil {
		t.Skipf("directory has no Samba schema: %v", err)
	}
}

type posixIDsResult struct {
	User     ldap.User `json:"user"`
	Warnings []string  `json:"warnings"`
}

// TestIntegration_User_ChangePosixIDs covers the write and, more importantly,
// what the response says about what was *not* done.
//
// The directory takes the new numbers instantly. Every file the account owns, on
// every host, still carries the old ones, and nothing here can reach them — so a
// silent success would be the most misleading possible outcome.
func TestIntegration_User_ChangePosixIDs(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	user := createTestUser(t, env, token)
	t.Cleanup(func() {
		_, _ = doJSON(t, env, http.MethodDelete, userPath(user.DN), nil, token)
	})

	newUID := user.UIDNumber + 5000
	newGID := user.GIDNumber + 5000

	resp, body := doJSON(t, env, http.MethodPost, userPath(user.DN)+"/change-posix-ids",
		map[string]any{"uidNumber": newUID, "gidNumber": newGID}, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("change: status=%d body=%s", resp.StatusCode, body)
	}

	var result posixIDsResult
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, body)
	}
	if result.User.UIDNumber != newUID {
		t.Errorf("uidNumber = %d, want %d", result.User.UIDNumber, newUID)
	}
	if result.User.GIDNumber != newGID {
		t.Errorf("gidNumber = %d, want %d", result.User.GIDNumber, newGID)
	}

	// Read it back from the directory, not from the response the handler built.
	stored, err := env.LDAP.GetUser(user.DN)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if stored.UIDNumber != newUID || stored.GIDNumber != newGID {
		t.Errorf("stored uid/gid = %d/%d, want %d/%d",
			stored.UIDNumber, stored.GIDNumber, newUID, newGID)
	}

	// The whole point: the caller is told what is left to do elsewhere.
	if len(result.Warnings) == 0 {
		t.Fatal("no warnings: the response implies the change is complete, which it is not")
	}
	joined := strings.Join(result.Warnings, " ")
	for _, want := range []string{"chown", "every host"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings do not mention %q: %v", want, result.Warnings)
		}
	}
	// A primary group nobody has is legal and almost always a typo.
	if !strings.Contains(joined, "no group in this directory has gid") {
		t.Errorf("a gid belonging to no group was not flagged: %v", result.Warnings)
	}
}

// A uid number two accounts share is one user as far as the kernel is concerned:
// same files, same access, and nothing afterwards can tell them apart.
func TestIntegration_User_ChangePosixIDs_RefusesADuplicate(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	first := createTestUser(t, env, token)
	second := createTestUser(t, env, token)
	t.Cleanup(func() {
		for _, u := range []*ldap.User{first, second} {
			_, _ = doJSON(t, env, http.MethodDelete, userPath(u.DN), nil, token)
		}
	})

	resp, body := doJSON(t, env, http.MethodPost, userPath(second.DN)+"/change-posix-ids",
		map[string]any{"uidNumber": first.UIDNumber}, token)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status=%d, want 409 (body=%s)", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), first.UID) {
		t.Errorf("the message does not name the account already holding it: %s", body)
	}

	// And nothing was written.
	stored, err := env.LDAP.GetUser(second.DN)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if stored.UIDNumber == first.UIDNumber {
		t.Error("the duplicate was written anyway")
	}
}

func TestIntegration_User_ChangePosixIDs_Validation(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	user := createTestUser(t, env, token)
	t.Cleanup(func() {
		_, _ = doJSON(t, env, http.MethodDelete, userPath(user.DN), nil, token)
	})

	for _, tc := range []struct {
		name string
		body map[string]any
		want int
	}{
		{name: "below the minimum", body: map[string]any{"uidNumber": 12}, want: http.StatusBadRequest},
		{name: "nothing given", body: map[string]any{}, want: http.StatusBadRequest},
		{
			name: "already set",
			body: map[string]any{"uidNumber": user.UIDNumber, "gidNumber": user.GIDNumber},
			want: http.StatusBadRequest,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := doJSON(t, env, http.MethodPost,
				userPath(user.DN)+"/change-posix-ids", tc.body, token)
			if resp.StatusCode != tc.want {
				t.Errorf("status=%d, want %d (body=%s)", resp.StatusCode, tc.want, body)
			}
		})
	}

	// A read-only session may not do this at all.
	viewer := loginAs(t, env, "viewer", "viewer123").Token
	resp, _ := doJSON(t, env, http.MethodPost, userPath(user.DN)+"/change-posix-ids",
		map[string]any{"uidNumber": user.UIDNumber + 9000}, viewer)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("read-only status=%d, want 403", resp.StatusCode)
	}
}

// TestIntegration_User_ChangePosixIDs_AlignsSambaSID: a sambaSID whose RID came
// from the uid number stops describing the entry the moment that number changes.
// Realigned where the derivation is recognisable, left alone and reported where
// it is not — a SID is the identity Windows uses, and rewriting one that was
// allocated elsewhere would hand the account a different principal.
func TestIntegration_User_ChangePosixIDs_AlignsSambaSID(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	// The bundled directory does not load the Samba schema, so sambaSamAccount is
	// undefined there and no entry can carry a SID. Skipped rather than dropped:
	// the behaviour is real wherever Samba is configured, and this runs there.
	requireSambaSchema(t, env, token)

	t.Run("derived from the uid number", func(t *testing.T) {
		user := createTestUser(t, env, token)
		t.Cleanup(func() { _, _ = doJSON(t, env, http.MethodDelete, userPath(user.DN), nil, token) })

		sid := fmt.Sprintf("S-1-5-21-0-0-0-%d", user.UIDNumber)
		if _, err := env.LDAP.SetSambaUserAttributes(user.DN,
			ldap.UpdateSambaUserRequest{SambaSID: &sid}); err != nil {
			t.Fatalf("set sambaSID: %v", err)
		}

		newUID := user.UIDNumber + 7000
		resp, body := doJSON(t, env, http.MethodPost, userPath(user.DN)+"/change-posix-ids",
			map[string]any{"uidNumber": newUID}, token)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("change: status=%d body=%s", resp.StatusCode, body)
		}

		stored, err := env.LDAP.GetUser(user.DN)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		want := fmt.Sprintf("S-1-5-21-0-0-0-%d", newUID)
		if stored.SambaSID != want {
			t.Errorf("sambaSID = %q, want %q", stored.SambaSID, want)
		}

		var result posixIDsResult
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		// Reported even though it succeeded: a Windows client holding the old SID
		// sees a different account, which is not something to discover later.
		if !strings.Contains(strings.Join(result.Warnings, " "), "realigned") {
			t.Errorf("the realignment was not reported: %v", result.Warnings)
		}
	})

	t.Run("allocated elsewhere", func(t *testing.T) {
		user := createTestUser(t, env, token)
		t.Cleanup(func() { _, _ = doJSON(t, env, http.MethodDelete, userPath(user.DN), nil, token) })

		sid := "S-1-5-21-111-222-333-513"
		if _, err := env.LDAP.SetSambaUserAttributes(user.DN,
			ldap.UpdateSambaUserRequest{SambaSID: &sid}); err != nil {
			t.Fatalf("set sambaSID: %v", err)
		}

		resp, body := doJSON(t, env, http.MethodPost, userPath(user.DN)+"/change-posix-ids",
			map[string]any{"uidNumber": user.UIDNumber + 7000}, token)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("change: status=%d body=%s", resp.StatusCode, body)
		}

		stored, err := env.LDAP.GetUser(user.DN)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if stored.SambaSID != sid {
			t.Errorf("sambaSID = %q, want it untouched at %q", stored.SambaSID, sid)
		}

		var result posixIDsResult
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !strings.Contains(strings.Join(result.Warnings, " "), "left alone") {
			t.Errorf("leaving the SID alone was not reported: %v", result.Warnings)
		}
	})
}
