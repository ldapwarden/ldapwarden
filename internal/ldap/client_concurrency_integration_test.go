//go:build integration

package ldap

import (
	"sync"
	"testing"
	"time"
)

// TestConcurrentRequestsOnSharedConnection is the regression test for requests
// that hung for the whole LDAP timeout and then failed, at random, whenever two
// arrived together — which every page that loads more than one thing does.
//
// connect() used to re-bind the shared connection on every call, to check it was
// alive. A bind resets the connection's state, and the server abandons whatever
// operations are still in progress on it (RFC 4511 §4.2.1) without answering
// them: a search started by one request was dropped by the next request's bind,
// and its caller waited for a reply that never came.
func TestConcurrentRequestsOnSharedConnection(t *testing.T) {
	client := netgroupTestClient(t)
	client.config.Timeout = 5 * time.Second

	const callers = 40
	var wg sync.WaitGroup
	errs := make(chan error, callers*5)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 5 {
				if _, err := client.Search(client.UserBaseDN(), "(objectClass=inetOrgPerson)", []string{"uid"}); err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent search failed: %v", err)
	}
}
