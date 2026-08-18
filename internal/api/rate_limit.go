package api

import (
	"net"
	"net/http"
	"time"

	"github.com/go-chi/httprate"
)

// Rate-limit budgets per client IP. Hardcoded for now; lift them into
// LDAPWARDEN_RATELIMIT_* env vars if an operator ever needs different numbers.
const (
	loginPerMinute        = 10
	resetGetPerMinute     = 20
	resetPostPer15Minutes = 5
)

// keyByClientIP buckets requests by the client address, coarsened to a /64 for
// IPv6 so a single client cannot walk its own prefix for a fresh budget.
//
// It reads r.RemoteAddr, which trustedProxyRealIPMiddleware has already resolved
// to the real client: behind a reverse proxy it rewrites RemoteAddr from
// X-Forwarded-For, but only when the TCP peer is in the configured
// trusted_proxies CIDRs, and it leaves RemoteAddr untouched otherwise. That is
// what makes RemoteAddr the right key here — the naive alternatives are each
// broken one way or the other. Keying off the raw peer address puts every client
// behind a proxy in one shared bucket, where a single attacker starves everyone
// else's login budget; keying off the header unconditionally lets a remote
// attacker forge the key, evading the limit by rotating it or locking a victim
// out by pinning it (the flaw behind httprate's KeyByRealIP deprecation and
// chi's middleware.RealIP advisories).
//
// This middleware must therefore stay mounted after trustedProxyRealIPMiddleware
// in the chain — see routes.go.
func keyByClientIP(r *http.Request) (string, error) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	return httprate.CanonicalizeIP(ip), nil
}

// loginRateLimit caps POST /api/auth/login attempts. Failed logins also
// pass through here, which is the goal: an attacker stuffing credentials
// against the LDAP bind hits the limit and gets 429s — the failures are
// audit-logged regardless so operators can correlate with the 429 responses.
func loginRateLimit() func(http.Handler) http.Handler {
	return httprate.LimitBy(loginPerMinute, time.Minute, keyByClientIP)
}

// passwordResetGetRateLimit caps GET /api/password-reset/{token}, which is
// cheap (single SELECT) but still worth shielding from token-fishing.
func passwordResetGetRateLimit() func(http.Handler) http.Handler {
	return httprate.LimitBy(resetGetPerMinute, time.Minute, keyByClientIP)
}

// passwordResetPostRateLimit caps the finalise-password call. The token
// itself is single-use after success, so the budget here protects against
// concurrent guessing more than against valid-token replay.
func passwordResetPostRateLimit() func(http.Handler) http.Handler {
	return httprate.LimitBy(resetPostPer15Minutes, 15*time.Minute, keyByClientIP)
}
