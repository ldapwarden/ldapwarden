package api

import (
	"net/http"

	"github.com/ldapwarden/ldapwarden/internal/audit"
)

func (s *Server) handleGetSchema(w http.ResponseWriter, r *http.Request) {
	schema, err := s.ldapClient.GetSchema()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch schema: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, schema)
}

func (s *Server) handleRefreshSchema(w http.ResponseWriter, r *http.Request) {
	schema, err := s.ldapClient.GetSchema()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to refresh schema: "+err.Error())
		return
	}

	s.auditLogger.Log(r.Context(), audit.ActionSchemaRefresh, audit.ResourceSchema, "", nil)

	writeJSON(w, http.StatusOK, schema)
}
