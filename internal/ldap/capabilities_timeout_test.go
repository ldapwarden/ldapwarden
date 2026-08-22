package ldap

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/ldapwarden/ldapwarden/internal/config"
)

// blackholeListener accepts connections and then says nothing at all, which is
// what a firewall dropping an idle connection, or a server wedged mid-request,
// looks like from the client side: the TCP dial succeeds and the reply never
// comes. Reproducing it needs no LDAP server.
func blackholeListener(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		var held []net.Conn
		defer func() {
			for _, c := range held {
				_ = c.Close()
			}
		}()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Hold it open and never write: closing would give the client an
			// error, which is precisely what this test needs not to happen.
			held = append(held, conn)
		}
	}()

	return fmt.Sprintf("ldap://%s", ln.Addr().String())
}

func blackholeClient(t *testing.T, timeout time.Duration) *Client {
	t.Helper()
	return NewClient(config.LDAPConfig{
		URL:      blackholeListener(t),
		BaseDN:   "dc=example,dc=org",
		BindDN:   "cn=admin,dc=example,dc=org",
		BindPass: "admin",
		Timeout:  timeout,
	})
}

// TestCapabilitiesBoundedAgainstUnresponsiveServer is the regression test for a
// dashboard that never filled in.
//
// Capabilities probes the directory twice, and used to do it with capMu held and
// with no deadline on either search. Against a server that accepted the
// connection and never answered, the first caller blocked for ever — and because
// the HTTP handlers carry no context, the browser giving up did not unblock it.
// The goroutine kept the mutex, so every later caller blocked too, for the
// lifetime of the process. What that looked like in the product: the stat cards
// and the expiring-accounts panel stuck on their skeletons and the nav falling
// back to its default sections, while the audit log and the scheduled-task list
// — Postgres, not LDAP — carried on as though nothing were wrong.
func TestCapabilitiesBoundedAgainstUnresponsiveServer(t *testing.T) {
	const timeout = 200 * time.Millisecond
	client := blackholeClient(t, timeout)
	t.Cleanup(client.Close)

	done := make(chan Capabilities, 1)
	go func() { done <- client.Capabilities() }()

	// Two probes, each bounded by the connection deadline. A generous multiple
	// of that still fails loudly if the bound is not applied at all.
	select {
	case caps := <-done:
		if caps.ScheduledExpiry {
			t.Error("ScheduledExpiry is true against a server that never answered")
		}
		if caps.PolicyEnforcement != EnforcementUnknown {
			t.Errorf("PolicyEnforcement = %q, want %q against an unreachable directory",
				caps.PolicyEnforcement, EnforcementUnknown)
		}
	case <-time.After(20 * timeout):
		t.Fatal("Capabilities did not return: the probe is unbounded again")
	}
}

// TestCapabilitiesConcurrentCallersShareOneProbe pins the other half of the fix.
//
// Bounding the probe stops it hanging for ever, but with the mutex still held
// across the searches N concurrent callers would each run their own probe in
// turn, so the wait grew with the number of requests in flight. They now wait on
// the one probe already running.
func TestCapabilitiesConcurrentCallersShareOneProbe(t *testing.T) {
	const (
		timeout = 200 * time.Millisecond
		callers = 16
	)
	client := blackholeClient(t, timeout)
	t.Cleanup(client.Close)

	var wg sync.WaitGroup
	start := make(chan struct{})
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			client.Capabilities()
		}()
	}

	began := time.Now()
	close(start)
	wg.Wait()
	elapsed := time.Since(began)

	// Serialised, this would be callers * 2 probes * timeout — 6.4s here. Shared,
	// it is one probe's worth. The bound sits well clear of both.
	if limit := 8 * timeout; elapsed > limit {
		t.Errorf("%d concurrent callers took %v (limit %v): each one is probing separately",
			callers, elapsed, limit)
	}
}
