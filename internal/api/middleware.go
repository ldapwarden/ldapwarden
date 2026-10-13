package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	ldaplib "github.com/go-ldap/ldap/v3"

	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/auth"
	"github.com/ldapwarden/ldapwarden/internal/ldap"
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

	// A base entry the directory does not hold: the deployment's configuration
	// and its directory disagree, and every request against that collection will
	// fail the same way until an operator acts. Answered like the other "this
	// directory cannot serve this" case — see netgroupsAvailable — and naming the
	// setting to change, because "internal server error" sent people to file a
	// bug for something an ldapadd fixes. The setting name is safe to return; the
	// DN it expands to is not, and is left in the log.
	if missing, ok := ldap.AsMissingBase(err); ok {
		writeError(w, http.StatusNotImplemented, fmt.Sprintf(
			"This directory has no %s base entry, so %s cannot be served. "+
				"Check %s, or create the entry in the directory.",
			missing.Collection, missing.Collection, missing.Setting))
		return
	}

	// A POSIX number already in use. Refused rather than written, because two
	// accounts on one uidNumber are one user as far as the kernel is concerned —
	// same files, same access, and nothing afterwards can tell them apart. The
	// holder is named: the caller has to pick another number, and knowing which
	// entry has it is how they do that.
	var taken *ldap.ErrPosixIDTaken
	if errors.As(err, &taken) {
		writeError(w, http.StatusConflict, fmt.Sprintf(
			"%s number %d already belongs to %q. Two entries sharing one are the same %s "+
				"as far as the system is concerned.",
			taken.Kind, taken.Number, taken.Holder,
			map[string]string{"uid": "user", "gid": "group"}[taken.Kind]))
		return
	}

	// Directory-side errors that the operator can actually act on get a clear,
	// curated message and a 4xx instead of an opaque 500 — but only a fixed
	// message keyed off the LDAP result code, never the raw error text (which
	// can carry DNs and server internals the redaction policy keeps out of
	// responses).
	if msg, status, ok := ldapErrorResponse(err); ok {
		writeError(w, status, msg)
		return
	}

	msg := "internal server error"
	if reqID != "" {
		msg += " (requestId=" + reqID + ")"
	}
	writeError(w, http.StatusInternalServerError, msg)
}

// ldapErrorResponse maps a directory error to a client-facing message and HTTP
// status. It returns ok=false for anything that isn't a recognised LDAP result
// code, so the caller falls back to a generic 500.
func ldapErrorResponse(err error) (string, int, bool) {
	var ldapErr *ldaplib.Error
	if !errors.As(err, &ldapErr) {
		return "", 0, false
	}
	switch ldapErr.ResultCode {
	case ldaplib.LDAPResultObjectClassViolation,
		ldaplib.LDAPResultUndefinedAttributeType,
		ldaplib.LDAPResultInvalidAttributeSyntax:
		// Typically the required schema (e.g. the sudo schema) is not loaded on
		// the directory, so an object class or attribute is unknown.
		return "The directory rejected this change: a required object class or attribute is missing or invalid. " +
			"The corresponding LDAP schema may not be loaded on the server — ask your directory administrator.", http.StatusUnprocessableEntity, true
	// NoSuchObject is deliberately left unmapped: it is ambiguous (missing
	// parent OU on create vs. an already-deleted target on update/delete) and a
	// single message would mislead for one of those cases, so it falls through
	// to the generic 500.
	case ldaplib.LDAPResultEntryAlreadyExists:
		return "An entry with this name already exists in the directory.", http.StatusConflict, true
	case ldaplib.LDAPResultInsufficientAccessRights:
		return "The directory denied this operation. The service account may lack permission to make this change.", http.StatusForbidden, true
	case ldaplib.LDAPResultConstraintViolation:
		return "The directory rejected this value as violating a constraint (for example a uniqueness or value rule).", http.StatusUnprocessableEntity, true
	default:
		return "", 0, false
	}
}

// writePasswordError is writeServerError for a password write.
//
// ppolicy refuses a password that breaks the policy with constraintViolation —
// the same result code the directory uses for a uniqueness or a syntax rule — so
// the generic mapping answered "violating a constraint (for example a uniqueness
// or value rule)". True, and useless: the operator learns neither the minimum
// length, nor the history depth, nor the minimum age, and can only guess at a
// value that will be accepted. Name the applicable policy's rules instead.
func (s *Server) writePasswordError(w http.ResponseWriter, r *http.Request, action, dn string, err error) {
	var ldapErr *ldaplib.Error
	if errors.As(err, &ldapErr) && ldapErr.ResultCode == ldaplib.LDAPResultConstraintViolation {
		msg := "The directory refused this password: it breaks the password policy that applies to this account."
		if rules := passwordRules(s.ldapClient.EffectivePasswordPolicy(dn)); rules != "" {
			msg += " " + rules
		}
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	writeServerError(w, r, action, err)
}

// passwordRules renders the parts of a policy an operator can act on, as one
// sentence. Empty when there is no policy to read or it sets none of them.
func passwordRules(policy *ldap.PasswordPolicy) string {
	if policy == nil {
		return ""
	}

	var rules []string
	if policy.PwdMinLength > 0 {
		rules = append(rules, fmt.Sprintf("at least %d characters", policy.PwdMinLength))
	}
	if policy.PwdInHistory > 0 {
		rules = append(rules, fmt.Sprintf("not one of the last %d passwords used", policy.PwdInHistory))
	}
	if policy.PwdCheckQuality > 0 {
		// The directory decides what "quality" means (pwdCheckModule, or slapd's
		// own check), so describe the requirement rather than inventing rules.
		rules = append(rules, "complex enough for the directory's quality check")
	}
	if len(rules) == 0 {
		return ""
	}
	// A colon rather than "requires": the clauses are fragments, and only a
	// colon reads correctly for every combination of them.
	return "Policy " + policy.CN + ": " + joinClauses(rules) + "."
}

// joinClauses renders a list as prose: "a", "a and b", "a, b and c". The "and"
// belongs to the join, not to any one clause — hardcoding it onto the last rule
// produced "requires and complex enough…" whenever that rule was the only one
// the policy set.
func joinClauses(clauses []string) string {
	switch len(clauses) {
	case 0:
		return ""
	case 1:
		return clauses[0]
	default:
		return strings.Join(clauses[:len(clauses)-1], ", ") + " and " + clauses[len(clauses)-1]
	}
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
		//nolint:gosec // G706: the DN is attacker-influenced but printed with %q,
		// which escapes newlines and control characters, so it cannot forge a log
		// line.
		log.Printf("session revocation FAILED (sessions may survive): reason=%q dn=%q requestId=%s err=%v",
			reason, dn, reqID, err)
	}
}

// revokeResetTokens retires the pending password-reset links of dn after a
// change that makes them unsafe to honour (see passwordreset.RevokeForUser).
// Like invalidateSessions it runs after the mutation committed, so a failure is
// logged loudly rather than failing the request.
func (s *Server) revokeResetTokens(r *http.Request, dn, reason string) {
	if err := s.passwordReset.RevokeForUser(r.Context(), dn); err != nil {
		reqID := middleware.GetReqID(r.Context())
		//nolint:gosec // G706: printed with %q, as in invalidateSessions.
		log.Printf("reset-token revocation FAILED (a pending reset link may survive): reason=%q dn=%q requestId=%s err=%v",
			reason, dn, reqID, err)
	}
}

// revokeAccess ends everything that still lets someone act as dn without its
// current credentials: live sessions and pending reset links. For the changes
// that take an account away from whoever held it — delete, rename, lock, a
// password set or removed by an admin.
func (s *Server) revokeAccess(r *http.Request, dn, reason string) {
	s.invalidateSessions(r, dn, reason)
	s.revokeResetTokens(r, dn, reason)
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
	if err := s.auditLogger.LogWithActor(r.Context(), actorDN, actorUID, "", action, resourceType, resourceDN, details); err != nil {
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
