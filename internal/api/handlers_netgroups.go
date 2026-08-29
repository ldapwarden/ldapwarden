package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// netgroupsAvailable refuses every netgroup route on a directory that does not
// declare nisNetgroup.
//
// Without it the endpoints would answer with an empty list on such a directory,
// which reads as "there are none" rather than "this server cannot hold any" —
// and a create would fail at the write with an object-class violation instead of
// saying so up front.
func (s *Server) netgroupsAvailable(w http.ResponseWriter) bool {
	if s.ldapClient.Capabilities().Netgroups {
		return true
	}
	writeError(w, http.StatusNotImplemented,
		"This directory does not declare the nisNetgroup object class, so it cannot hold netgroups. "+
			"Load nis.schema on the server to enable them.")
	return false
}

func (s *Server) handleListNetgroups(w http.ResponseWriter, r *http.Request) {
	if !s.netgroupsAvailable(w) {
		return
	}
	opts := parseListOptions(r)

	result, err := s.ldapClient.SearchNetgroupsPage(opts)
	if err != nil {
		writeServerError(w, r, "list netgroups", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":      result.Items,
		"total":     result.Total,
		"limit":     opts.Limit,
		"offset":    opts.Offset,
		"truncated": result.Truncated,
	})
}

func (s *Server) handleGetNetgroup(w http.ResponseWriter, r *http.Request) {
	if !s.netgroupsAvailable(w) {
		return
	}
	dn, err := resolveDN(r, s.ldapClient.NetgroupBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	netgroup, err := s.ldapClient.GetNetgroup(dn)
	if err != nil {
		writeError(w, http.StatusNotFound, "netgroup not found")
		return
	}

	writeJSON(w, http.StatusOK, netgroup)
}

// handleExpandNetgroup resolves nesting and answers with what the netgroup
// actually contributes.
//
// A separate endpoint rather than a field on the entry: expanding reads the
// whole collection, and a listing that expanded every row would pay for it once
// per row.
func (s *Server) handleExpandNetgroup(w http.ResponseWriter, r *http.Request) {
	if !s.netgroupsAvailable(w) {
		return
	}
	dn, err := resolveDN(r, s.ldapClient.NetgroupBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	netgroup, err := s.ldapClient.GetNetgroup(dn)
	if err != nil {
		writeError(w, http.StatusNotFound, "netgroup not found")
		return
	}

	expansion, err := s.ldapClient.ExpandNetgroup(netgroup.CN)
	if err != nil {
		writeServerError(w, r, "expand netgroup", err)
		return
	}

	writeJSON(w, http.StatusOK, expansion)
}

// handleGetUserNetgroups lists the netgroups that reach this user.
//
// Direct membership and membership through nesting are both included and
// distinguished: "+parent" in a sudo rule grants to everyone the parent
// absorbs, so a netgroup the user is only in indirectly still matters, but it
// is not the one to edit to remove them.
func (s *Server) handleGetUserNetgroups(w http.ResponseWriter, r *http.Request) {
	if !s.netgroupsAvailable(w) {
		return
	}
	dn, err := resolveDN(r, s.ldapClient.UserBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	user, err := s.ldapClient.GetUser(dn)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	// One call, because both halves of the answer come from one graph. Asking
	// for the effective membership and the direct membership separately read the
	// whole netgroup collection twice per request, to derive the second from a
	// subset of what the first had already loaded.
	memberships, err := s.ldapClient.NetgroupMembershipsForUser(user.UID)
	if err != nil {
		writeServerError(w, r, "list user netgroups", err)
		return
	}
	if memberships == nil {
		memberships = []ldap.NetgroupMembership{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": memberships})
}

func (s *Server) handleCreateNetgroup(w http.ResponseWriter, r *http.Request) {
	if !s.netgroupsAvailable(w) {
		return
	}
	var req ldap.CreateNetgroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := validateRDNValue("cn", req.CN); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateTriples(req.Triples); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	plannedDN := "cn=" + req.CN + "," + s.ldapClient.NetgroupBaseDN()
	if !s.auditMutating(w, r, audit.ActionNetgroupCreate, audit.ResourceNetgroup, plannedDN,
		map[string]interface{}{"cn": req.CN}) {
		return
	}

	netgroup, err := s.ldapClient.CreateNetgroup(req)
	if err != nil {
		writeServerError(w, r, "create netgroup", err)
		return
	}

	writeJSON(w, http.StatusCreated, netgroup)
}

func (s *Server) handleUpdateNetgroup(w http.ResponseWriter, r *http.Request) {
	if !s.netgroupsAvailable(w) {
		return
	}
	dn, err := resolveDN(r, s.ldapClient.NetgroupBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	var req ldap.UpdateNetgroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateTriples(req.Triples); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Values the parser could not read are carried over rather than dropped: a
	// save about the triples the client can see must not delete the ones it
	// cannot. The client never sends them, so they are read here.
	current, err := s.ldapClient.GetNetgroup(dn)
	if err != nil {
		writeError(w, http.StatusNotFound, "netgroup not found")
		return
	}
	req.KeepMalformed = current.Malformed

	if !s.auditMutating(w, r, audit.ActionNetgroupUpdate, audit.ResourceNetgroup, dn, nil) {
		return
	}

	netgroup, err := s.ldapClient.UpdateNetgroup(dn, req)
	if err != nil {
		writeServerError(w, r, "update netgroup", err)
		return
	}

	writeJSON(w, http.StatusOK, netgroup)
}

func (s *Server) handleDeleteNetgroup(w http.ResponseWriter, r *http.Request) {
	if !s.netgroupsAvailable(w) {
		return
	}
	dn, err := resolveDN(r, s.ldapClient.NetgroupBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	current, err := s.ldapClient.GetNetgroup(dn)
	if err != nil {
		writeError(w, http.StatusNotFound, "netgroup not found")
		return
	}

	if !s.auditMutating(w, r, audit.ActionNetgroupDelete, audit.ResourceNetgroup, dn, nil) {
		return
	}

	// What will be left pointing at nothing. Sudo names a netgroup by "+name",
	// and nothing in the directory ties that string to this entry, so a delete
	// leaves those rules matching nobody — silently, which is the failure mode
	// this whole feature has to keep saying out loud. Collected before the
	// delete, while the name still resolves, and reported rather than refused:
	// removing a netgroup is a legitimate thing to do.
	warnings := []string{}
	if roles, rerr := s.ldapClient.GetNetgroupSudoRoles(current.CN); rerr != nil {
		warnings = append(warnings, "could not check which sudo roles name this netgroup: "+rerr.Error())
	} else {
		for _, role := range roles {
			warnings = append(warnings, fmt.Sprintf(
				"sudo role %q still names +%s, which now matches nobody", role.CN, current.CN))
		}
	}
	if nesting, nerr := s.ldapClient.NetgroupsNesting(current.CN); nerr != nil {
		warnings = append(warnings, "could not check which netgroups nest this one: "+nerr.Error())
	} else {
		for _, name := range nesting {
			warnings = append(warnings, fmt.Sprintf(
				"netgroup %q still nests %q, which no longer exists", name, current.CN))
		}
	}

	if err := s.ldapClient.DeleteNetgroup(dn); err != nil {
		writeServerError(w, r, "delete netgroup", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "netgroup deleted",
		"warnings": warnings,
	})
}

// validateTriples refuses "(,,)", which is legal in the schema and almost
// certainly a mistake: every slot open means every user on every host, a grant
// nobody types on purpose. The directory accepts it, so it is caught here.
//
// Nothing else is refused. A triple with the host and user the wrong way round
// is indistinguishable from a deliberate one — both are just names — so the UI
// warns about the shape instead of the API rejecting it.
func validateTriples(triples []ldap.NetgroupTriple) error {
	for _, triple := range triples {
		if triple.IsEmpty() {
			return fmt.Errorf("a netgroup triple cannot have all three fields empty: " +
				"that matches every user on every host")
		}
	}
	return nil
}
