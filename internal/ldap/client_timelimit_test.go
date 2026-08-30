package ldap

import (
	"testing"
	"time"
)

// TestSearchTimeLimitSeconds: the point of the value is that it is never the
// protocol's "no limit", and never longer than the connection deadline it sits
// inside — a server that gives up and says so beats a client deadline expiring
// on a connection with nothing to report.
func TestSearchTimeLimitSeconds(t *testing.T) {
	cases := []struct {
		timeout time.Duration
		want    int
	}{
		{timeout: 30 * time.Second, want: 29},
		{timeout: 2 * time.Second, want: 1},
		{timeout: 1 * time.Second, want: 1},
		{timeout: 500 * time.Millisecond, want: 1},
		{timeout: 0, want: 1},
		{timeout: -5 * time.Second, want: 1},
	}
	for _, tc := range cases {
		if got := searchTimeLimitSeconds(tc.timeout); got != tc.want {
			t.Errorf("searchTimeLimitSeconds(%s) = %d, want %d", tc.timeout, got, tc.want)
		}
		if got := searchTimeLimitSeconds(tc.timeout); got < 1 {
			t.Errorf("searchTimeLimitSeconds(%s) = %d, which the protocol reads as no limit",
				tc.timeout, got)
		}
	}
}
