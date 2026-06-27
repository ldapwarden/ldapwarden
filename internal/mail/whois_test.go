package mail

import (
	"net/netip"
	"testing"
)

// TestIsGlobalUnicast guards the address classification that gates the whois
// lookup: only globally-routable unicast addresses may reach the outbound
// request. The non-global cases below (CGNAT, ULA, link-local, unspecified)
// are exactly the ones a naive string-prefix check used to miss.
func TestIsGlobalUnicast(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"2606:4700:4700::1111", true},
		{"10.0.0.1", false},          // RFC 1918
		{"192.168.1.1", false},       // RFC 1918
		{"172.16.0.1", false},        // RFC 1918
		{"127.0.0.1", false},         // loopback
		{"169.254.1.1", false},       // link-local
		{"100.64.0.1", false},        // RFC 6598 CGNAT
		{"100.127.255.255", false},   // RFC 6598 CGNAT upper bound
		{"0.0.0.0", false},           // unspecified
		{"224.0.0.1", false},         // multicast
		{"::1", false},               // IPv6 loopback
		{"fc00::1", false},           // IPv6 ULA
		{"fe80::1", false},           // IPv6 link-local
		{"::", false},                // IPv6 unspecified
	}

	for _, c := range cases {
		addr, err := netip.ParseAddr(c.ip)
		if err != nil {
			t.Fatalf("parse %q: %v", c.ip, err)
		}
		if got := isGlobalUnicast(addr); got != c.want {
			t.Errorf("isGlobalUnicast(%s)=%v, want %v", c.ip, got, c.want)
		}
	}
}
