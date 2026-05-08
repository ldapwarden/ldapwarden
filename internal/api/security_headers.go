package api

import "net/http"

// contentSecurityPolicy is rendered as one line per response. Note:
//   - 'unsafe-inline' in style-src is required because login.tsx uses inline
//     style={{}} attributes for animated gradients, and Vite production builds
//     can emit inline <style> chunks. Tailwind itself compiles to classes and
//     does not need this.
//   - data: in img-src enables avatar rendering (jpegPhoto is base64'd into a
//     data URI in src/components/ui/avatar.tsx).
//   - frame-ancestors 'none' supersedes X-Frame-Options on modern browsers;
//     X-Frame-Options is still emitted below for legacy clients.
const contentSecurityPolicy = "default-src 'self'; " +
	"img-src 'self' data:; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"font-src 'self' data:; " +
	"connect-src 'self'; " +
	"object-src 'none'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'"

// secureHeadersMiddleware applies a baseline set of HTTP security headers to
// every response. HSTS is intentionally only emitted when the request reached
// the server over TLS — when terminating TLS at a reverse proxy, configure
// HSTS there instead, otherwise a misconfigured proxy chain could pin clients
// to a hostname that does not yet have a valid certificate.
func secureHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		if r.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}
