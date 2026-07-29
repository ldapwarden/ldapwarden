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

// csrfOriginCheck refuses any cookie-authenticated mutation whose Origin (or,
// falling back, Referer) is neither this server's own origin nor one of the
// configured LDAPWARDEN_CORS_ORIGINS. SameSite=Lax on the session cookie
// already blocks most CSRF vectors, but legitimate browsers always send Origin
// on non-safe requests, so verifying it costs nothing and closes the niche
// cases SameSite=Lax doesn't (e.g. simple cross-origin POST with a permissive
// cookie policy upstream).
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

// originAllowed reports whether the request's Origin (or Referer fallback) is
// this server's own origin, or is in the allowlist. Returns false on
// missing/unparseable headers — a legitimate browser always sends one of them on
// a non-safe request.
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

	// A request whose Origin is this very server is not cross-origin, whatever
	// the allowlist says: the page that sent it was served from here. No other
	// site can forge that header, so accepting it costs nothing — and refusing
	// it broke the normal deployment, where the binary serves the SPA and the
	// API from one origin: every mutation came back "cross-origin request
	// refused" until LDAPWARDEN_CORS_ORIGINS happened to name that origin.
	if own := requestOrigin(r); own != "" && own == candidate {
		return true
	}

	for _, a := range allowed {
		if strings.TrimRight(a, "/") == candidate {
			return true
		}
	}
	return false
}

// requestOrigin reconstructs the origin the request was addressed to, i.e. what
// a browser would send as Origin for a page served by this server.
//
// The scheme comes from X-Forwarded-Proto when a trusted proxy supplied one (TLS
// is terminated upstream, so r.TLS is nil even though the client used https),
// otherwise from the connection itself. The authority is r.Host — the name the
// client actually used. X-Forwarded-Host is deliberately ignored: it widens the
// spoofable surface, and a proxy that rewrites Host is exactly the case
// LDAPWARDEN_CORS_ORIGINS exists for.
func requestOrigin(r *http.Request) string {
	if r.Host == "" {
		return ""
	}

	scheme := "http"
	if proto := forwardedProtoFromContext(r.Context()); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}

	return scheme + "://" + r.Host
}
