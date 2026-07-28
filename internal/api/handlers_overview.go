package api

import "net/http"

// handleGetOverview returns the directory at a glance for the dashboard: user
// population, locked and expiring accounts, and the size of each collection.
//
// Gated on users:read, which both roles hold — the figures are aggregates of
// data any authenticated operator can already list.
func (s *Server) handleGetOverview(w http.ResponseWriter, r *http.Request) {
	overview, err := s.ldapClient.GetOverview()
	if err != nil {
		writeServerError(w, r, "get overview", err)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}
