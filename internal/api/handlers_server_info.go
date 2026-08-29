package api

import (
	"net/http"

	"github.com/ldapwarden/ldapwarden/internal/buildinfo"
)

// serverInfoResponse is what the UI needs to decide what to render, plus which
// build it is talking to.
type serverInfoResponse struct {
	Version buildinfo.Info `json:"version"`
	Modules []string       `json:"modules"`
	// The configured object classes, for the same reason as Modules: the user
	// and group pages decide which tabs exist from them, and a read-only session
	// that could not read them fell back to a hardcoded guess.
	UsersObjects  []string             `json:"usersObjects"`
	GroupsObjects []string             `json:"groupsObjects"`
	Capabilities  CapabilitiesResponse `json:"capabilities"`
}

// handleGetServerInfo answers for any authenticated session, with no permission
// required.
//
// The UI used to read all of this from /api/admin/config, which takes
// settings:read — a permission the readonly role does not have. So a read-only
// user, whom the API happily lets list netgroups, was served a UI that hid the
// netgroup tab, the membership section and the sudo rule panels, because the
// capability probe behind them had 403'd. Nothing said so; the feature simply
// was not there.
//
// None of this is privileged. Which modules are on and what the directory can
// hold is already evident from using the app, and the version has to reach a
// footer every user sees. What is privileged — bind DNs, connection strings,
// masked secrets — stays in /api/admin/config, which keeps its permission.
//
// Not on /healthz either: that route is unauthenticated, and publishing an exact
// version to anyone who can reach the port is free reconnaissance against an
// administration tool.
func (s *Server) handleGetServerInfo(w http.ResponseWriter, r *http.Request) {
	capabilities := s.ldapClient.Capabilities()

	writeJSON(w, http.StatusOK, serverInfoResponse{
		Version:       buildinfo.Get(),
		Modules:       s.config.App.Modules,
		UsersObjects:  s.config.App.UsersObjects,
		GroupsObjects: s.config.App.GroupsObjects,
		Capabilities: CapabilitiesResponse{
			ScheduledExpirySupported: capabilities.ScheduledExpiry,
			PolicyEnforcement:        string(capabilities.PolicyEnforcement),
			NetgroupsSupported:       capabilities.Netgroups,
		},
	})
}
