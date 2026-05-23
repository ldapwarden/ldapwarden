package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
