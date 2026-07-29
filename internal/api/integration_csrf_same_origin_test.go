//go:build integration

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// TestIntegration_CSRF_SameOriginWriteAllowed exercises the exact path a browser
// takes — session cookie, no Authorization header, Origin set to the origin that
// served the page — against the real middleware chain.
//
// The unit tests cover the decision; this covers the wiring, which is where it
// went wrong before: the app served its own SPA, so every mutation carried an
// Origin that the allowlist did not mention, and the documented compose
// quick-start could not write anything.
func TestIntegration_CSRF_SameOriginWriteAllowed(t *testing.T) {
	env := setupTestServer(t)

	// The test server's own origin, which is not in LDAPWARDEN_CORS_ORIGINS
	// (that default names the dev-server ports).
	origin := env.Server.URL

	// Log in for the Set-Cookie, and keep the cookie only: with an
	// Authorization header the request would take the bearer path, which is
	// exempt from the origin check.
	body, err := json.Marshal(map[string]string{"username": "admin", "password": "admin123"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	loginReq, err := http.NewRequest(http.MethodPost, origin+"/api/auth/login", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.Header.Set("Origin", origin)

	loginResp, err := http.DefaultClient.Do(loginReq)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	_ = loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login: status=%d", loginResp.StatusCode)
	}

	var sessionCookie *http.Cookie
	for _, c := range loginResp.Cookies() {
		if c.Name == sessionCookieName {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatalf("login set no %s cookie", sessionCookieName)
	}

	// A user to mutate. Created with the bearer token, which is not what is
	// under test here.
	token := loginAs(t, env, "admin", "admin123").Token
	user := createTestUser(t, env, token)

	cookieWrite := func(originHeader string) int {
		t.Helper()
		payload, err := json.Marshal(map[string]string{"title": "csrf probe"})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		req, err := http.NewRequest(http.MethodPut, origin+userPath(user.DN), bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(sessionCookie)
		if originHeader != "" {
			req.Header.Set("Origin", originHeader)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("write: %v", err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	if got := cookieWrite(origin); got != http.StatusOK {
		t.Errorf("same-origin cookie write: status=%d, want 200", got)
	}
	if got := cookieWrite("https://attacker.example"); got != http.StatusForbidden {
		t.Errorf("cross-origin cookie write: status=%d, want 403", got)
	}
	if got := cookieWrite(""); got != http.StatusForbidden {
		t.Errorf("cookie write with no Origin: status=%d, want 403", got)
	}
}
