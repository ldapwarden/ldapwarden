//go:build integration

package api

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/ldapwarden/ldapwarden/internal/auth"
)

// TestIntegration_SessionStore covers the PostgreSQL session store that
// replaced Redis. What Redis gave for free has to hold here by construction:
// an expired session is gone the moment it expires, not when the purge next
// runs, and revoking a user's sessions removes all of them.
func TestIntegration_SessionStore(t *testing.T) {
	env := setupTestServer(t)
	store := auth.NewPostgresSessionStore(env.Pool)
	ctx := context.Background()

	dn := "uid=zz-session-" + uniqueSuffix(t) + ",ou=People,dc=example,dc=org"
	other := dn + "-other"
	t.Cleanup(func() {
		_, _ = env.Pool.Exec(ctx, `DELETE FROM sessions WHERE user_dn IN ($1, $2)`, dn, other)
	})

	newSession := func(userDN string, expiresIn time.Duration) *auth.Session {
		return &auth.Session{
			ID: uniqueSuffix(t), UserDN: userDN, UserUID: "zz", RoleName: "readonly",
			Permissions: []string{"users:read"}, ExpiresAt: time.Now().Add(expiresIn),
		}
	}

	live := newSession(dn, time.Hour)
	if err := store.Create(ctx, live, "hash-live-"+live.ID); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := store.Get(ctx, "hash-live-"+live.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.UserDN != dn || len(got.Permissions) != 1 || got.Permissions[0] != "users:read" {
		t.Errorf("round trip lost data: %+v", got)
	}

	// Expired but not purged: must already read as absent.
	expired := newSession(dn, -time.Minute)
	if err := store.Create(ctx, expired, "hash-expired-"+expired.ID); err != nil {
		t.Fatalf("create expired: %v", err)
	}
	if _, err := store.Get(ctx, "hash-expired-"+expired.ID); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Errorf("expired session: err = %v, want ErrSessionNotFound", err)
	}

	purged, err := store.DeleteExpired(ctx)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if purged < 1 {
		t.Errorf("purge removed %d rows, want the expired one at least", purged)
	}
	if _, err := store.Get(ctx, "hash-live-"+live.ID); err != nil {
		t.Errorf("the purge removed a live session: %v", err)
	}

	// Revoking one user's sessions: all of them, and nobody else's.
	second := newSession(dn, time.Hour)
	bystander := newSession(other, time.Hour)
	for hash, s := range map[string]*auth.Session{"hash-second-" + second.ID: second, "hash-bystander-" + bystander.ID: bystander} {
		if err := store.Create(ctx, s, hash); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	if err := store.DeleteByUserDN(ctx, dn); err != nil {
		t.Fatalf("delete by user: %v", err)
	}
	for _, hash := range []string{"hash-live-" + live.ID, "hash-second-" + second.ID} {
		if _, err := store.Get(ctx, hash); !errors.Is(err, auth.ErrSessionNotFound) {
			t.Errorf("%s survived the revocation: err = %v", hash, err)
		}
	}
	if _, err := store.Get(ctx, "hash-bystander-"+bystander.ID); err != nil {
		t.Errorf("another user's session was revoked too: %v", err)
	}
}

// A login, a request on the session and a logout, end to end through the HTTP
// stack, now that nothing but PostgreSQL holds the session.
func TestIntegration_SessionLifecycleWithoutRedis(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "viewer", "viewer123").Token

	resp, body := doJSON(t, env, http.MethodGet, "/api/auth/me", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("me: status=%d body=%s", resp.StatusCode, body)
	}
	resp, body = doJSON(t, env, http.MethodPost, "/api/auth/logout", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout: status=%d body=%s", resp.StatusCode, body)
	}
	resp, _ = doJSON(t, env, http.MethodGet, "/api/auth/me", nil, token)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("me after logout: status=%d, want 401", resp.StatusCode)
	}
}
