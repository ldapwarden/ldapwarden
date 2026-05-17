//go:build integration

package api

import (
	"context"
	"testing"
	"time"

	"github.com/ldapwarden/ldapwarden/internal/passwordreset"
)

// TestIntegration_PasswordReset_SupersedesPriorTokens asserts that
// CreateToken retires any unused-and-still-valid token for the same
// user. A user ends up with at most one usable reset link, which caps
// the blast radius of token leakage.
func TestIntegration_PasswordReset_SupersedesPriorTokens(t *testing.T) {
	env := setupTestServer(t)
	svc := passwordreset.NewService(env.Pool)

	userDN := "uid=throttled-" + uniqueSuffix(t) + ",ou=People,dc=example,dc=org"
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = env.Pool.Exec(ctx, `DELETE FROM password_reset_tokens WHERE user_dn=$1`, userDN)
	})

	first, err := svc.CreateToken(ctx, userDN, "throttled", "t@example.org",
		"uid=admin,ou=People,dc=example,dc=org")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}

	if _, err := svc.ValidateToken(ctx, first); err != nil {
		t.Fatalf("first token did not validate immediately: %v", err)
	}

	if _, err = svc.CreateToken(ctx, userDN, "throttled", "t@example.org",
		"uid=admin,ou=People,dc=example,dc=org"); err != nil {
		t.Fatalf("create second: %v", err)
	}

	// The first token must now be unusable.
	if _, err := svc.ValidateToken(ctx, first); err == nil {
		t.Errorf("first token still validates after second issuance; want superseded")
	}
}

// TestIntegration_PasswordReset_CountersTrackRecentIssuance covers the
// CountRecentByActor / CountRecentForUser helpers that the handler uses
// to enforce per-actor and per-target caps.
func TestIntegration_PasswordReset_CountersTrackRecentIssuance(t *testing.T) {
	env := setupTestServer(t)
	svc := passwordreset.NewService(env.Pool)

	actorDN := "uid=counter-admin-" + uniqueSuffix(t) + ",ou=People,dc=example,dc=org"
	userA := "uid=counter-a-" + uniqueSuffix(t) + ",ou=People,dc=example,dc=org"
	userB := "uid=counter-b-" + uniqueSuffix(t) + ",ou=People,dc=example,dc=org"
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = env.Pool.Exec(ctx, `DELETE FROM password_reset_tokens WHERE created_by_dn=$1`, actorDN)
	})

	mustCreate := func(target string) {
		if _, err := svc.CreateToken(ctx, target, "u", "u@example.org", actorDN); err != nil {
			t.Fatalf("create token for %s: %v", target, err)
		}
	}
	mustCreate(userA)
	mustCreate(userA) // supersedes the prior; still counts as an issuance
	mustCreate(userB)

	n, err := svc.CountRecentByActor(ctx, actorDN, time.Minute)
	if err != nil {
		t.Fatalf("CountRecentByActor: %v", err)
	}
	if n != 3 {
		t.Errorf("CountRecentByActor=%d, want 3", n)
	}

	n, err = svc.CountRecentForUser(ctx, userA, time.Minute)
	if err != nil {
		t.Fatalf("CountRecentForUser: %v", err)
	}
	if n != 2 {
		t.Errorf("CountRecentForUser(A)=%d, want 2", n)
	}

	// A window in the past should see nothing.
	n, err = svc.CountRecentByActor(ctx, actorDN, -1*time.Minute)
	if err != nil {
		t.Fatalf("CountRecentByActor(past window): %v", err)
	}
	if n != 0 {
		t.Errorf("CountRecentByActor(past window)=%d, want 0", n)
	}
}
