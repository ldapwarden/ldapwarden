package api

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// ParseTrustedProxies turns a slice of CIDR strings into parsed networks.
// Returns an error on the first invalid entry so callers (cmd/server) can
// fail fast at startup instead of silently degrading the security model.
func ParseTrustedProxies(cidrs []string) ([]*net.IPNet, error) {
	if len(cidrs) == 0 {
		return nil, nil
	}
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", c, err)
		}
		out = append(out, n)
	}
	return out, nil
}

// trustedProxyRealIPMiddleware rewrites r.RemoteAddr from
// True-Client-IP / X-Real-IP / X-Forwarded-For — but only when the immediate
// peer (the connection-level RemoteAddr) belongs to one of the trusted CIDRs.
// When trusted is empty, headers are never honoured: that's the safe default
// for a server exposed directly to the Internet, since otherwise any client
// can inject an audit-log IP of its choosing.
//
// This replaces chi's middleware.RealIP, which honoured the headers
// unconditionally.
func trustedProxyRealIPMiddleware(trusted []*net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if peerIsTrusted(r.RemoteAddr, trusted) {
				if v := realIPFromHeaders(r); v != "" {
					r.RemoteAddr = v
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// peerIsTrusted reports whether the connection-level remote (host:port string
// as provided by net/http) lies within any of the trusted networks.
func peerIsTrusted(remoteAddr string, trusted []*net.IPNet) bool {
	if len(trusted) == 0 {
		return false
	}
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// realIPFromHeaders extracts a client IP from forwarded-for-style headers,
// matching the precedence chi's middleware.RealIP used. Returns "" when no
// header carries a parseable IP.
func realIPFromHeaders(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("True-Client-IP")); v != "" && net.ParseIP(v) != nil {
		return v
	}
	if v := strings.TrimSpace(r.Header.Get("X-Real-IP")); v != "" && net.ParseIP(v) != nil {
		return v
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		// X-Forwarded-For can be a comma-separated chain; the first entry
		// is the original client.
		if i := strings.IndexByte(v, ','); i >= 0 {
			v = v[:i]
		}
		v = strings.TrimSpace(v)
		if net.ParseIP(v) != nil {
			return v
		}
	}
	return ""
}
