package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCSRFOriginCheck_SameOrigin covers the normal deployment: the binary serves
// the SPA and the API from one origin, so the browser's Origin is this server's
// own. That must be accepted whatever the allowlist holds — otherwise every
// mutation is refused until LDAPWARDEN_CORS_ORIGINS happens to name the URL the
// operator uses, which is how the documented compose quick-start ended up unable
// to write anything.
func TestCSRFOriginCheck_SameOrigin(t *testing.T) {
	// Empty allowlist on purpose: nothing here may depend on it.
	h := csrfOriginCheck(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	trustedProxy := "10.0.0.5:4444"
	trusted, err := ParseTrustedProxies([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatalf("parse trusted proxies: %v", err)
	}

	cases := []struct {
		name string
		// target drives r.Host and, for an https target, r.TLS.
		target         string
		origin         string
		referer        string
		remoteAddr     string
		forwardedProto string
		want           int
	}{
		{
			name:   "plain HTTP, served and called on the same host",
			target: "http://ldap.example.com:8000/api/users",
			origin: "http://ldap.example.com:8000",
			want:   http.StatusOK,
		},
		{
			name:   "direct TLS",
			target: "https://ldap.example.com/api/users",
			origin: "https://ldap.example.com",
			want:   http.StatusOK,
		},
		{
			name:    "same origin via Referer",
			target:  "http://ldap.example.com:8000/api/users",
			referer: "http://ldap.example.com:8000/users",
			want:    http.StatusOK,
		},
		{
			// TLS terminated at a reverse proxy: r.TLS is nil, the client used
			// https, and only X-Forwarded-Proto says so.
			name:           "TLS terminated by a trusted proxy",
			target:         "http://ldap.example.com/api/users",
			origin:         "https://ldap.example.com",
			remoteAddr:     trustedProxy,
			forwardedProto: "https",
			want:           http.StatusOK,
		},
		{
			// Same headers from an untrusted peer must not be believed, or any
			// client could claim a scheme to match its own Origin.
			name:           "forwarded scheme from an untrusted peer is ignored",
			target:         "http://ldap.example.com/api/users",
			origin:         "https://ldap.example.com",
			remoteAddr:     "203.0.113.9:5555",
			forwardedProto: "https",
			want:           http.StatusForbidden,
		},
		{
			name:   "another host on the same scheme is still cross-origin",
			target: "http://ldap.example.com:8000/api/users",
			origin: "http://attacker.example.com:8000",
			want:   http.StatusForbidden,
		},
		{
			name:   "same host on a different port is still cross-origin",
			target: "http://ldap.example.com:8000/api/users",
			origin: "http://ldap.example.com:9000",
			want:   http.StatusForbidden,
		},
		{
			name:   "opaque origin refused",
			target: "http://ldap.example.com:8000/api/users",
			origin: "null",
			want:   http.StatusForbidden,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.target, nil)
			req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "abcd"})
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.referer != "" {
				req.Header.Set("Referer", tc.referer)
			}
			if tc.remoteAddr != "" {
				req.RemoteAddr = tc.remoteAddr
			}
			if tc.forwardedProto != "" {
				req.Header.Set("X-Forwarded-Proto", tc.forwardedProto)
			}

			rec := httptest.NewRecorder()
			// Through the forwarded-headers middleware, which is what decides
			// whether X-Forwarded-Proto may be believed.
			trustedProxyRealIPMiddleware(trusted)(h).ServeHTTP(rec, req)

			if rec.Code != tc.want {
				t.Errorf("status=%d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestCSRFOriginCheck(t *testing.T) {
	allowed := []string{"https://app.example.com", "http://localhost:5173"}

	cases := []struct {
		name    string
		method  string
		cookie  bool   // whether to attach a session cookie
		origin  string // Origin header, "" to omit
		referer string // Referer header, "" to omit
		want    int    // wanted status
	}{
		{name: "safe method bypasses check", method: http.MethodGet, cookie: true, want: http.StatusOK},
		{name: "no cookie bypasses check (bearer path)", method: http.MethodPost, cookie: false, want: http.StatusOK},
		{name: "cookie + matching origin allowed", method: http.MethodPost, cookie: true, origin: "https://app.example.com", want: http.StatusOK},
		{name: "cookie + trailing slash matches", method: http.MethodPost, cookie: true, origin: "https://app.example.com/", want: http.StatusOK},
		{name: "cookie + foreign origin refused", method: http.MethodPost, cookie: true, origin: "https://attacker.example.com", want: http.StatusForbidden},
		{name: "cookie + no origin/referer refused", method: http.MethodPost, cookie: true, want: http.StatusForbidden},
		{name: "cookie + matching referer accepted", method: http.MethodPost, cookie: true, referer: "https://app.example.com/users", want: http.StatusOK},
		{name: "cookie + foreign referer refused", method: http.MethodPost, cookie: true, referer: "https://attacker.example.com/users", want: http.StatusForbidden},
		{name: "cookie + DELETE foreign origin refused", method: http.MethodDelete, cookie: true, origin: "https://attacker.example.com", want: http.StatusForbidden},
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := csrfOriginCheck(allowed)(next)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "https://app.example.com/api/users", nil)
			if tc.cookie {
				req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "abcd"})
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.referer != "" {
				req.Header.Set("Referer", tc.referer)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("status=%d, want %d", rec.Code, tc.want)
			}
		})
	}
}
