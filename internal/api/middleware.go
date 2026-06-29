package api

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/auth"
)

// auditRequestInfoMiddleware attaches the caller's IP address and User-Agent
// to the request context so audit log entries (and their notification emails)
// capture them. Must be mounted after chi's middleware.RealIP, which already
// normalises r.RemoteAddr to honour X-Forwarded-For / X-Real-IP.
func auditRequestInfoMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}
		ctx := audit.ContextWithRequestInfo(r.Context(), audit.RequestInfo{
			IPAddress: ip,
			UserAgent: r.Header.Get("User-Agent"),
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// tokenFromRequest extracts a session token from the HttpOnly session
// cookie if present, falling back to a Bearer Authorization header.
// Cookie takes precedence so an old Authorization header lingering in a
// proxy or browser extension cannot resurrect a logged-out session. Bearer
// remains supported for machine clients (scripts, monitoring probes) that
// have no cookie jar.
func tokenFromRequest(r *http.Request) string {
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	authHeader := r.Header.Get("Authorization")
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return ""
	}
	return parts[1]
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := tokenFromRequest(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing session token")
			return
		}

		session, err := s.authService.ValidateToken(r.Context(), token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		ctx := auth.ContextWithSession(r.Context(), session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) requirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !s.rbac.HasPermission(r.Context(), permission) {
				writeError(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// writeServerError logs the underlying error against the request ID and
// writes a generic 500 response. The original err — which often carries
// LDAP / pgx / SMTP diagnostics, library versions or internal paths —
// never reaches the client; operators correlate by request ID via stderr.
// `action` is the high-level verb that failed (e.g. "delete user") and is
// the only context echoed back so support can sanity-check the report.
func writeServerError(w http.ResponseWriter, r *http.Request, action string, err error) {
	reqID := middleware.GetReqID(r.Context())
	log.Printf("server error: action=%q requestId=%s err=%v", action, reqID, err)
	msg := "internal server error"
	if reqID != "" {
		msg += " (requestId=" + reqID + ")"
	}
	writeError(w, http.StatusInternalServerError, msg)
}

// invalidateSessions revokes every live session for dn after a
// security-relevant mutation (lock, delete, password change, admin-group
// change). The mutation has already committed by the time this runs, so a
// revocation failure cannot fail the HTTP request — but it is logged loudly
// rather than swallowed, because a session that survives for a locked, deleted
// or demoted user is a security gap an operator needs to act on.
func (s *Server) invalidateSessions(r *http.Request, dn, reason string) {
	if err := s.authService.InvalidateUserSessions(r.Context(), dn); err != nil {
		reqID := middleware.GetReqID(r.Context())
		log.Printf("session revocation FAILED (sessions may survive): reason=%q dn=%q requestId=%s err=%v",
			reason, dn, reqID, err)
	}
}

// Request-body size caps. defaultMaxBodyBytes covers every JSON payload the
// API exchanges except user create/update, which carries a base64
// jpegPhoto. Applied per route group rather than globally because chi
// middleware ordering makes outer caps win, so a per-route LARGER cap
// can't loosen an outer one.
const (
	defaultMaxBodyBytes int64 = 1 * 1024 * 1024  // 1 MiB
	photoMaxBodyBytes   int64 = 10 * 1024 * 1024 // 10 MiB
)

// maxBodyBytes returns a middleware that caps the request body at n bytes
// using http.MaxBytesReader. The wrapped reader returns an error past the
// limit, which json.Decode surfaces as a 400. Without this an attacker
// could buffer hundreds of MB into RAM by streaming a giant JSON body.
func maxBodyBytes(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}

// auditMutating writes an audit entry for an action that is about to mutate
// LDAP state, and refuses to proceed when the insert fails. Callers MUST
// invoke it before calling the LDAP client and `return` when it returns
// false (a 500 response has already been written).
//
// Recording the intent first keeps the audit trail authoritative when the
// audit DB hiccups: we refuse the change rather than letting it land
// untracked. If the subsequent LDAP op fails the audit row records an
// attempt that did not complete — preferable to a completed change with no
// record at all.
func (s *Server) auditMutating(w http.ResponseWriter, r *http.Request, action audit.Action, resourceType audit.ResourceType, resourceDN string, details map[string]interface{}) bool {
	if err := s.auditLogger.Log(r.Context(), action, resourceType, resourceDN, details); err != nil {
		writeError(w, http.StatusInternalServerError, "audit unavailable; refusing to mutate")
		return false
	}
	return true
}

// auditMutatingWithActor is the variant for endpoints that mutate state
// without an authenticated session in the context (currently the password
// reset confirmation flow). Same contract as auditMutating.
func (s *Server) auditMutatingWithActor(w http.ResponseWriter, r *http.Request, actorDN, actorUID string, action audit.Action, resourceType audit.ResourceType, resourceDN string, details map[string]interface{}) bool {
	if err := s.auditLogger.LogWithActor(r.Context(), actorDN, actorUID, action, resourceType, resourceDN, details); err != nil {
		writeError(w, http.StatusInternalServerError, "audit unavailable; refusing to mutate")
		return false
	}
	return true
}

type PaginatedResponse struct {
	Data   interface{} `json:"data"`
	Total  int64       `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}
