package api

import (
	"net/http"
	"net/url"
	"strings"
)

// safeMethods are exempt from origin-check: per RFC 9110 they MUST NOT
// have side effects, so a cross-site forgery of one is not a CSRF.
var safeMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodHead:    {},
	http.MethodOptions: {},
}

// csrfOriginCheck refuses any cookie-authenticated mutation whose Origin
// (or, falling back, Referer) does not match one of the configured
// LDAPWARDEN_CORS_ORIGINS. SameSite=Lax on the session cookie already
// blocks most CSRF vectors, but legitimate browsers always send Origin on
// non-safe requests, so verifying it costs nothing and closes the niche
// cases SameSite=Lax doesn't (e.g. simple cross-origin POST with a
// permissive cookie policy upstream).
//
// Bearer-authenticated requests are exempt: a machine client with the
// raw token has no cookie jar to abuse, the Origin header is typically
// absent, and the request cannot have been triggered by a browser
// drive-by.
func csrfOriginCheck(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, safe := safeMethods[r.Method]; safe {
				next.ServeHTTP(w, r)
				return
			}
			// Bearer-only auth (no session cookie) cannot be a CSRF.
			if _, err := r.Cookie(sessionCookieName); err != nil {
				next.ServeHTTP(w, r)
				return
			}
			if !originAllowed(r, allowedOrigins) {
				writeError(w, http.StatusForbidden, "cross-origin request refused")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// originAllowed reports whether the request's Origin (or Referer fallback)
// is in the allowlist. Returns false on missing/unparseable headers — a
// legitimate browser always sends one of them on a non-safe request.
func originAllowed(r *http.Request, allowed []string) bool {
	candidate := r.Header.Get("Origin")
	if candidate == "" {
		ref := r.Header.Get("Referer")
		if ref == "" {
			return false
		}
		u, err := url.Parse(ref)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return false
		}
		candidate = u.Scheme + "://" + u.Host
	}
	candidate = strings.TrimRight(candidate, "/")
	for _, a := range allowed {
		if strings.TrimRight(a, "/") == candidate {
			return true
		}
	}
	return false
}
