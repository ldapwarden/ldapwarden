package api

import (
	"net/http"
	"time"
)

// sessionCookieName is the cookie that carries the bearer session token.
// It is HttpOnly so the browser keeps JS away from it, mitigating the
// XSS-equivalent risk we previously had by storing the token in
// localStorage.
const sessionCookieName = "ldapwarden_session"

// setSessionCookie writes the session token into an HttpOnly cookie.
// SameSite=Lax allows normal navigation (clicking a link back to the app
// keeps the user logged in) but blocks the typical CSRF vector of a
// third-party form posting to our endpoints. Secure is conditional on
// dev mode so the bundled compose stack on plain http:// still works;
// real deployments are expected to terminate TLS in front of the app.
func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(s.config.Session.TTL / time.Second),
		HttpOnly: true,
		Secure:   !s.config.App.DevMode,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearSessionCookie expires the session cookie immediately. Called from
// the logout handler so the browser drops the value even though the
// underlying token has already been invalidated server-side.
func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   !s.config.App.DevMode,
		SameSite: http.SameSiteLaxMode,
	})
}
