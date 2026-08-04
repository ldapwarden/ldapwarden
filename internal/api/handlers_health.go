package api

import (
	"context"
	"net/http"
	"time"
)

// readinessTimeout bounds each dependency check. A probe that hangs is a probe
// that tells the orchestrator nothing, so a slow dependency is reported as not
// ready rather than waited on.
const readinessTimeout = 3 * time.Second

// healthResponse is the body of both probes. Checks is omitted from the liveness
// answer, which deliberately has nothing to report.
type healthResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// handleLiveness answers "is this process running". Nothing more, on purpose: a
// liveness probe that fails because the directory is slow gets the container
// killed and restarted, which fixes nothing and drops the sessions of everyone
// using it. Dependency state belongs in readiness.
func (s *Server) handleLiveness(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

// handleReadiness answers "can this process serve traffic", by checking the three
// things it cannot work without.
//
// 503 when any of them is unreachable, so a load balancer or a readinessProbe
// stops sending requests that would only come back as 500s. The body names the
// failing dependency and quotes the error: this is unauthenticated, but an
// operator diagnosing an outage needs it, and anyone who can reach this endpoint
// can already infer as much from the 500s the API is returning.
func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]string, 3)
	ready := true

	record := func(name string, err error) {
		if err != nil {
			checks[name] = err.Error()
			ready = false
			return
		}
		checks[name] = "ok"
	}

	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()

	record("database", s.auditLogger.Ping(ctx))
	record("sessions", s.authService.PingSessionStore(ctx))
	// The LDAP client has no context-aware call; the dial timeout bounds a server
	// that is down, and a server that is hung is caught by the probe's own timeout
	// on the caller's side.
	record("directory", s.ldapClient.Ping())

	status := http.StatusOK
	state := "ready"
	if !ready {
		status = http.StatusServiceUnavailable
		state = "degraded"
	}
	writeJSON(w, status, healthResponse{Status: state, Checks: checks})
}
