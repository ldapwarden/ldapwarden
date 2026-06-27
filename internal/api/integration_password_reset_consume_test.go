//go:build integration

package api

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/passwordreset"
)

const consumeTestActor = "uid=admin,ou=People,dc=example,dc=org"

// TestIntegration_PasswordReset_ConsumeTokenSingleUse asserts that a token can
// be redeemed at most once: the second ConsumeToken fails and the token no
// longer validates. This is the sequential half of the single-use guarantee.
func TestIntegration_PasswordReset_ConsumeTokenSingleUse(t *testing.T) {
	env := setupTestServer(t)
	svc := passwordreset.NewService(env.Pool)

	userDN := "uid=consume-once-" + uniqueSuffix(t) + ",ou=People,dc=example,dc=org"
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = env.Pool.Exec(ctx, `DELETE FROM password_reset_tokens WHERE user_dn=$1`, userDN)
	})

	token, err := svc.CreateToken(ctx, userDN, "consume-once", "c@example.org", consumeTestActor)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	claimed, err := svc.ConsumeToken(ctx, token, "203.0.113.1")
	if err != nil {
		t.Fatalf("first ConsumeToken failed: %v", err)
	}
	if claimed.UserDN != userDN {
		t.Errorf("claimed.UserDN=%q, want %q", claimed.UserDN, userDN)
	}

	if _, err := svc.ConsumeToken(ctx, token, "203.0.113.2"); err == nil {
		t.Errorf("second ConsumeToken succeeded; want single-use failure")
	}
	if _, err := svc.ValidateToken(ctx, token); err == nil {
		t.Errorf("token still validates after being consumed")
	}
}

// TestIntegration_PasswordReset_ConsumeTokenIsAtomic races many goroutines
// against a single fresh token. Exactly one must win — this is the regression
// guard for the validate-then-mark race that the atomic
// `UPDATE ... WHERE used = FALSE RETURNING` closes.
func TestIntegration_PasswordReset_ConsumeTokenIsAtomic(t *testing.T) {
	env := setupTestServer(t)
	svc := passwordreset.NewService(env.Pool)

	userDN := "uid=consume-race-" + uniqueSuffix(t) + ",ou=People,dc=example,dc=org"
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = env.Pool.Exec(ctx, `DELETE FROM password_reset_tokens WHERE user_dn=$1`, userDN)
	})

	token, err := svc.CreateToken(ctx, userDN, "consume-race", "c@example.org", consumeTestActor)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	const racers = 24
	var (
		wg        sync.WaitGroup
		successes int64
		start     = make(chan struct{})
	)
	wg.Add(racers)
	for i := 0; i < racers; i++ {
		go func(i int) {
			defer wg.Done()
			<-start // line everyone up so the calls actually contend
			if _, err := svc.ConsumeToken(ctx, token, fmt.Sprintf("203.0.113.%d", i)); err == nil {
				atomic.AddInt64(&successes, 1)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	if successes != 1 {
		t.Errorf("concurrent ConsumeToken succeeded %d times; want exactly 1", successes)
	}
}
