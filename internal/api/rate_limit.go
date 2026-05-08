package api

import (
	"net/http"
	"time"

	"github.com/go-chi/httprate"
)

// Rate-limit budgets per remote IP. Hardcoded for now; if an operator runs
// LDAP Warden behind a shared proxy that aggregates many clients on one
// source IP, lift these into LDAPWARDEN_RATELIMIT_* env vars at that point.
const (
	loginPerMinute        = 10
	resetGetPerMinute     = 20
	resetPostPer15Minutes = 5
)

// loginRateLimit caps POST /api/auth/login attempts. Failed logins also
// pass through here, which is the goal: an attacker stuffing credentials
// against the LDAP bind hits the limit and gets 429s — the failures are
// audit-logged regardless so operators can correlate with the 429 responses.
func loginRateLimit() func(http.Handler) http.Handler {
	return httprate.LimitByIP(loginPerMinute, time.Minute)
}

// passwordResetGetRateLimit caps GET /api/password-reset/{token}, which is
// cheap (single SELECT) but still worth shielding from token-fishing.
func passwordResetGetRateLimit() func(http.Handler) http.Handler {
	return httprate.LimitByIP(resetGetPerMinute, time.Minute)
}

// passwordResetPostRateLimit caps the finalise-password call. The token
// itself is single-use after success, so the budget here protects against
// concurrent guessing more than against valid-token replay.
func passwordResetPostRateLimit() func(http.Handler) http.Handler {
	return httprate.LimitByIP(resetPostPer15Minutes, 15*time.Minute)
}
