package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/ldapwarden/ldapwarden/internal/audit"
)

// parseAuditFilters reads the filters shared by the listing and the CSV export,
// so an export always covers exactly the view it was taken from.
func parseAuditFilters(r *http.Request) audit.ListParams {
	q := r.URL.Query()
	params := audit.ListParams{
		ActorDN:      q.Get("actorDn"),
		ActorUID:     q.Get("actorUid"),
		ResourceType: audit.ResourceType(q.Get("resourceType")),
		Action:       audit.Action(q.Get("action")),
		Search:       q.Get("search"),
	}

	// Dates arrive as plain days (YYYY-MM-DD) from the date pickers. `to` covers
	// the whole day, otherwise picking the same day for both bounds would match
	// only entries logged at exactly midnight.
	if from, err := time.Parse(time.DateOnly, q.Get("from")); err == nil {
		params.From = from
	}
	if to, err := time.Parse(time.DateOnly, q.Get("to")); err == nil {
		params.To = to.Add(24*time.Hour - time.Nanosecond)
	}

	return params
}

func (s *Server) handleListAuditLogs(w http.ResponseWriter, r *http.Request) {
	params := parseAuditFilters(r)
	params.Limit = 50

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

// handleListAuditActors returns the actors present in the log so the UI can
// offer them as a filter instead of asking the operator to type an exact uid.
func (s *Server) handleListAuditActors(w http.ResponseWriter, r *http.Request) {
	actors, err := s.auditLogger.Actors(r.Context())
	if err != nil {
		writeServerError(w, r, "list audit actors", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": actors})
}

// handleExportAuditLogs streams the filtered log as CSV — the format an auditor
// asking for "the changes to this account last quarter" can actually work with.
// One row per entry, oldest first, with the structured diff flattened into a
// readable "field: old → new" column.
func (s *Server) handleExportAuditLogs(w http.ResponseWriter, r *http.Request) {
	params := parseAuditFilters(r)

	logs, err := s.auditLogger.Export(r.Context(), params)
	if err != nil {
		writeServerError(w, r, "export audit logs", err)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "audit-logs.csv"))
	// The log is append-only but the filtered slice is not: never cache it.
	w.Header().Set("Cache-Control", "no-store")

	cw := csv.NewWriter(w)
	defer cw.Flush()

	if err := cw.Write([]string{
		"timestamp", "actor_uid", "actor_dn", "action",
		"resource_type", "resource_dn", "changes", "details", "ip_address", "user_agent",
	}); err != nil {
		return
	}

	for _, entry := range logs {
		if err := cw.Write([]string{
			entry.CreatedAt.UTC().Format(time.RFC3339),
			entry.ActorUID,
			entry.ActorDN,
			string(entry.Action),
			string(entry.ResourceType),
			entry.ResourceDN,
			formatChanges(entry.Details),
			formatDetails(entry.Details),
			entry.IPAddress,
			entry.UserAgent,
		}); err != nil {
			return
		}
	}
}

// formatChanges renders details["changes"] as "field: old → new" lines. Masked
// changes (passwords) report the field only, never a value.
func formatChanges(details map[string]interface{}) string {
	raw, ok := details[audit.DetailsKeyChanges].([]interface{})
	if !ok {
		return ""
	}

	out := ""
	for _, item := range raw {
		change, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		field, _ := change["field"].(string)
		if masked, _ := change["masked"].(bool); masked {
			out += fmt.Sprintf("%s: (changed)\n", field)
			continue
		}
		old, _ := change["old"].(string)
		newValue, _ := change["new"].(string)
		out += fmt.Sprintf("%s: %s → %s\n", field, old, newValue)
	}
	return out
}

// formatDetails renders the non-diff part of details as compact JSON, so the
// CSV keeps whatever a given action recorded without duplicating the changes
// column.
func formatDetails(details map[string]interface{}) string {
	if len(details) == 0 {
		return ""
	}
	rest := make(map[string]interface{}, len(details))
	for k, v := range details {
		if k == audit.DetailsKeyChanges {
			continue
		}
		rest[k] = v
	}
	if len(rest) == 0 {
		return ""
	}
	encoded, err := json.Marshal(rest)
	if err != nil {
		return ""
	}
	return string(encoded)
}
