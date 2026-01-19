package api

import (
	"net/http"
	"strconv"

	"github.com/ldapwarden/ldapwarden/internal/audit"
)

func (s *Server) handleListAuditLogs(w http.ResponseWriter, r *http.Request) {
	params := audit.ListParams{
		Limit:        50,
		Offset:       0,
		ActorDN:      r.URL.Query().Get("actorDn"),
		ResourceType: audit.ResourceType(r.URL.Query().Get("resourceType")),
		Action:       audit.Action(r.URL.Query().Get("action")),
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			params.Limit = limit
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			params.Offset = offset
		}
	}

	logs, total, err := s.auditLogger.List(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list audit logs")
		return
	}

	writeJSON(w, http.StatusOK, PaginatedResponse{
		Data:   logs,
		Total:  total,
		Limit:  params.Limit,
		Offset: params.Offset,
	})
}
