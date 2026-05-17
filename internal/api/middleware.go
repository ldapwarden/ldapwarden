package api

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

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

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeError(w, http.StatusUnauthorized, "missing authorization header")
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			writeError(w, http.StatusUnauthorized, "invalid authorization header")
			return
		}

		token := parts[1]
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
