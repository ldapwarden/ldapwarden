package api

import (
	"net/http"

	"github.com/ldapwarden/ldapwarden/internal/audit"
)

func (s *Server) handleGetSchema(w http.ResponseWriter, r *http.Request) {
	schema, err := s.ldapClient.GetSchema()
	if err != nil {
		writeServerError(w, r, "fetch schema", err)
		return
	}

	writeJSON(w, http.StatusOK, schema)
}

func (s *Server) handleRefreshSchema(w http.ResponseWriter, r *http.Request) {
	// Re-probe capabilities too: loading an overlay is exactly the kind of change
	// an operator makes before hitting refresh, and pwdEndTime appears with it.
	s.ldapClient.RefreshCapabilities()

	schema, err := s.ldapClient.GetSchema()
	if err != nil {
		writeServerError(w, r, "refresh schema", err)
		return
	}

	_ = s.auditLogger.Log(r.Context(), audit.ActionSchemaRefresh, audit.ResourceSchema, "", nil)

	writeJSON(w, http.StatusOK, schema)
}
