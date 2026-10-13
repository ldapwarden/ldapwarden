//go:build integration

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
	"github.com/ldapwarden/ldapwarden/internal/passwordreset"
)

// The tests below cover the paths that change an account's credentials or
// rights without going through the dedicated endpoints already covered in
// integration_sessions_test.go — each used to leave sessions, or reset links,
// alive for their full lifetime.

// TestIntegration_Revocation_AdminGroupReplaceDropsSessions: replacing the admin
// group's whole member list (the group form's PUT) must end the session of every
// member it removes, as the per-member DELETE does.
func TestIntegration_Revocation_AdminGroupReplaceDropsSessions(t *testing.T) {
	env := setupTestServer(t)
	adminToken := loginAs(t, env, "admin", "admin123").Token
	adminGroupDN := "cn=" + env.Config.App.AdminGroup + "," + env.Config.LDAP.GroupOU + "," + env.Config.LDAP.BaseDN

	resp, body := doJSON(t, env, http.MethodGet, groupPath(adminGroupDN), nil, adminToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get admin group: status=%d body=%s", resp.StatusCode, body)
	}
	var group ldap.Group
	if err := json.Unmarshal(body, &group); err != nil {
		t.Fatalf("unmarshal group: %v", err)
	}
	original := group.MemberUIDs

	user := createTestUser(t, env, adminToken)
	promoted := append(append([]string{}, original...), user.UID)
	if resp, body := doJSON(t, env, http.MethodPut, groupPath(adminGroupDN),
		map[string]any{"memberUid": promoted}, adminToken); resp.StatusCode != http.StatusOK {
		t.Fatalf("promote via PUT: status=%d body=%s", resp.StatusCode, body)
	}
	t.Cleanup(func() {
		_, _ = doJSON(t, env, http.MethodPut, groupPath(adminGroupDN),
			map[string]any{"memberUid": original}, adminToken)
	})

	userToken := loginAs(t, env, user.UID, "testpass").Token
	if resp, _ := doJSON(t, env, http.MethodGet, "/api/auth/me", nil, userToken); resp.StatusCode != http.StatusOK {
		t.Fatalf("/me before demotion: status=%d, want 200", resp.StatusCode)
	}

	if resp, body := doJSON(t, env, http.MethodPut, groupPath(adminGroupDN),
		map[string]any{"memberUid": original}, adminToken); resp.StatusCode != http.StatusOK {
		t.Fatalf("demote via PUT: status=%d body=%s", resp.StatusCode, body)
	}

	if resp, _ := doJSON(t, env, http.MethodGet, "/api/auth/me", nil, userToken); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("/me after demotion via PUT: status=%d, want 401", resp.StatusCode)
	}
	// The admin making the change was not in the diff and keeps their session.
	if resp, _ := doJSON(t, env, http.MethodGet, "/api/auth/me", nil, adminToken); resp.StatusCode != http.StatusOK {
		t.Errorf("/me of the acting admin: status=%d, want 200", resp.StatusCode)
	}
}

// TestIntegration_Revocation_PasswordSetByUpdateDropsSessions: a password set
// through PUT /users/{dn} ends sessions like POST /users/{dn}/password does.
func TestIntegration_Revocation_PasswordSetByUpdateDropsSessions(t *testing.T) {
	env := setupTestServer(t)
	adminToken := loginAs(t, env, "admin", "admin123").Token
	user := createTestUser(t, env, adminToken)
	userToken := loginAs(t, env, user.UID, "testpass").Token

	if resp, body := doJSON(t, env, http.MethodPut, userPath(user.DN),
		map[string]any{"password": "Upd4ted-" + uniqueSuffix(t)}, adminToken); resp.StatusCode != http.StatusOK {
		t.Fatalf("update password: status=%d body=%s", resp.StatusCode, body)
	}

	if resp, _ := doJSON(t, env, http.MethodGet, "/api/auth/me", nil, userToken); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("/me after password set via PUT: status=%d, want 401", resp.StatusCode)
	}
}

// TestIntegration_Revocation_ResetConfirmDropsSessions: completing a reset ends
// the sessions opened with the old password.
func TestIntegration_Revocation_ResetConfirmDropsSessions(t *testing.T) {
	env := setupTestServer(t)
	adminToken := loginAs(t, env, "admin", "admin123").Token
	user := createTestUser(t, env, adminToken)
	userToken := loginAs(t, env, user.UID, "testpass").Token
	token := issueResetToken(t, env, user.DN, user.UID, user.Mail)

	if resp, body := doJSON(t, env, http.MethodPost, "/api/password-reset/"+token,
		map[string]string{"password": "R3set-" + uniqueSuffix(t)}, ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("confirm reset: status=%d body=%s", resp.StatusCode, body)
	}

	if resp, _ := doJSON(t, env, http.MethodGet, "/api/auth/me", nil, userToken); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("/me after reset: status=%d, want 401", resp.StatusCode)
	}
}

// TestIntegration_Revocation_ChangeOwnPassword: the current password is
// required and checked, and a change ends every session but the caller's.
func TestIntegration_Revocation_ChangeOwnPassword(t *testing.T) {
	env := setupTestServer(t)
	adminToken := loginAs(t, env, "admin", "admin123").Token
	user := createTestUser(t, env, adminToken)
	current := loginAs(t, env, user.UID, "testpass").Token
	other := loginAs(t, env, user.UID, "testpass").Token
	newPassword := "Ch4nged-" + uniqueSuffix(t)

	if resp, _ := doJSON(t, env, http.MethodPost, "/api/auth/change-password",
		map[string]string{"password": newPassword}, current); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("without currentPassword: status=%d, want 400", resp.StatusCode)
	}
	if resp, _ := doJSON(t, env, http.MethodPost, "/api/auth/change-password",
		map[string]string{"currentPassword": "wrong", "password": newPassword}, current); resp.StatusCode != http.StatusForbidden {
		t.Errorf("wrong currentPassword: status=%d, want 403", resp.StatusCode)
	}
	// Neither refusal may have touched the session or the password.
	if resp, _ := doJSON(t, env, http.MethodGet, "/api/auth/me", nil, other); resp.StatusCode != http.StatusOK {
		t.Fatalf("/me of other session after refusals: status=%d, want 200", resp.StatusCode)
	}

	if resp, body := doJSON(t, env, http.MethodPost, "/api/auth/change-password",
		map[string]string{"currentPassword": "testpass", "password": newPassword}, current); resp.StatusCode != http.StatusOK {
		t.Fatalf("change own password: status=%d body=%s", resp.StatusCode, body)
	}

	if resp, _ := doJSON(t, env, http.MethodGet, "/api/auth/me", nil, current); resp.StatusCode != http.StatusOK {
		t.Errorf("/me of the session that changed it: status=%d, want 200", resp.StatusCode)
	}
	if resp, _ := doJSON(t, env, http.MethodGet, "/api/auth/me", nil, other); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("/me of another session: status=%d, want 401", resp.StatusCode)
	}
	loginAs(t, env, user.UID, newPassword)
}

// TestIntegration_Revocation_ResetLinkRetired covers both halves of retiring a
// reset link: the in-app changes that revoke it, and the check against the
// entry that catches the changes made elsewhere.
func TestIntegration_Revocation_ResetLinkRetired(t *testing.T) {
	env := setupTestServer(t)
	adminToken := loginAs(t, env, "admin", "admin123").Token

	linkWorks := func(token string) bool {
		t.Helper()
		resp, _ := doJSON(t, env, http.MethodGet, "/api/password-reset/"+token, nil, "")
		return resp.StatusCode == http.StatusOK
	}

	t.Run("untouched account", func(t *testing.T) {
		user := createTestUser(t, env, adminToken)
		if !linkWorks(issueResetToken(t, env, user.DN, user.UID, user.Mail)) {
			t.Errorf("a link for an unchanged account must work")
		}
	})

	t.Run("lock", func(t *testing.T) {
		user := createTestUser(t, env, adminToken)
		token := issueResetToken(t, env, user.DN, user.UID, user.Mail)
		if resp, body := doJSON(t, env, http.MethodPost, userPath(user.DN)+"/lock", nil, adminToken); resp.StatusCode != http.StatusOK {
			t.Fatalf("lock: status=%d body=%s", resp.StatusCode, body)
		}
		if linkWorks(token) {
			t.Errorf("link still works after the account was locked")
		}
	})

	t.Run("password removed", func(t *testing.T) {
		user := createTestUser(t, env, adminToken)
		token := issueResetToken(t, env, user.DN, user.UID, user.Mail)
		if resp, body := doJSON(t, env, http.MethodDelete, userPath(user.DN)+"/password", nil, adminToken); resp.StatusCode != http.StatusOK {
			t.Fatalf("remove password: status=%d body=%s", resp.StatusCode, body)
		}
		if linkWorks(token) {
			t.Errorf("link still works after the password was removed")
		}
	})

	t.Run("mail changed", func(t *testing.T) {
		user := createTestUser(t, env, adminToken)
		token := issueResetToken(t, env, user.DN, user.UID, user.Mail)
		if resp, body := doJSON(t, env, http.MethodPut, userPath(user.DN),
			map[string]any{"mail": "moved-" + user.Mail}, adminToken); resp.StatusCode != http.StatusOK {
			t.Fatalf("change mail: status=%d body=%s", resp.StatusCode, body)
		}
		if linkWorks(token) {
			t.Errorf("link still works after the mail was changed")
		}
	})

	t.Run("mail unchanged on save", func(t *testing.T) {
		user := createTestUser(t, env, adminToken)
		token := issueResetToken(t, env, user.DN, user.UID, user.Mail)
		if resp, body := doJSON(t, env, http.MethodPut, userPath(user.DN),
			map[string]any{"mail": user.Mail, "description": "saved"}, adminToken); resp.StatusCode != http.StatusOK {
			t.Fatalf("save: status=%d body=%s", resp.StatusCode, body)
		}
		if !linkWorks(token) {
			t.Errorf("saving the form without changing the mail retired the link")
		}
	})

	// The token records a mail the entry no longer carries — what a change
	// made outside the app leaves behind, with nothing to revoke the link.
	t.Run("mail differs from the entry", func(t *testing.T) {
		user := createTestUser(t, env, adminToken)
		if linkWorks(issueResetToken(t, env, user.DN, user.UID, "old-"+user.Mail)) {
			t.Errorf("link works although the entry's mail is not the one it was sent to")
		}
	})
}

// issueResetToken creates a reset token straight in the store, as
// handleSendPasswordReset would, without needing a mail server.
func issueResetToken(t *testing.T, env *testEnv, dn, uid, mail string) string {
	t.Helper()
	ctx := context.Background()
	token, err := passwordreset.NewService(env.Pool).CreateToken(ctx, dn, uid, mail, consumeTestActor)
	if err != nil {
		t.Fatalf("create reset token: %v", err)
	}
	t.Cleanup(func() {
		_, _ = env.Pool.Exec(ctx, `DELETE FROM password_reset_tokens WHERE user_dn=$1`, dn)
	})
	return token
}
