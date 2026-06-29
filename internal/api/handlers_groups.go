package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// invalidateIfAdminGroupChange drops every session belonging to memberUID when
// groupDN refers to the configured admin group. Called from add/remove member
// handlers so a privilege grant takes effect on the user's next login (forcing
// re-derivation of permissions) and a privilege revocation takes effect
// immediately. Best-effort: if the user UID cannot be resolved to a DN, we
// skip silently — LDAP does not enforce that memberUid corresponds to an
// existing user (see TestIntegration_Groups_AddMember_UnknownUserStillSucceeds).
func (s *Server) invalidateIfAdminGroupChange(r *http.Request, groupDN, memberUID string) {
	cn := dnFirstRDNValue(groupDN)
	if !strings.EqualFold(cn, s.config.App.AdminGroup) {
		return
	}
	user, err := s.ldapClient.GetUserByUID(memberUID)
	if err != nil {
		return
	}
	s.invalidateSessions(r, user.DN, "admin-group membership change")
}

func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	groups, truncated, err := s.ldapClient.SearchGroups(r.URL.Query().Get("search"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list groups")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":      groups,
		"total":     len(groups),
		"truncated": truncated,
	})
}

func (s *Server) handleGetGroup(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.GroupBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	group, err := s.ldapClient.GetGroup(dn)
	if err != nil {
		writeError(w, http.StatusNotFound, "group not found")
		return
	}

	writeJSON(w, http.StatusOK, group)
}

func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var req ldap.CreateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := validateRDNValue("cn", req.CN); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := validatePOSIXID("gidNumber", req.GIDNumber, s.ldapClient.MinGID()); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	plannedDN := "cn=" + req.CN + "," + s.ldapClient.GroupBaseDN()
	details := map[string]interface{}{
		audit.DetailsKeyResourceName: req.CN,
		audit.DetailsKeyChanges:      groupCreateFields(req),
	}
	if !s.auditMutating(w, r, audit.ActionGroupCreate, audit.ResourceGroup, plannedDN, details) {
		return
	}

	group, err := s.ldapClient.CreateGroup(req)
	if err != nil {
		writeServerError(w, r, "create group", err)
		return
	}

	writeJSON(w, http.StatusCreated, group)
}

func (s *Server) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.GroupBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	var req ldap.UpdateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Capture the pre-update state for a field-level audit diff (best-effort).
	var details map[string]interface{}
	if before, err := s.ldapClient.GetGroup(dn); err == nil && before != nil {
		details = map[string]interface{}{audit.DetailsKeyResourceName: groupDisplayName(before)}
		if changes := groupUpdateChanges(before, req); len(changes) > 0 {
			details[audit.DetailsKeyChanges] = changes
		}
	}

	if !s.auditMutating(w, r, audit.ActionGroupUpdate, audit.ResourceGroup, dn, details) {
		return
	}

	group, err := s.ldapClient.UpdateGroup(dn, req)
	if err != nil {
		writeServerError(w, r, "update group", err)
		return
	}

	writeJSON(w, http.StatusOK, group)
}

func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.GroupBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	if !s.auditMutating(w, r, audit.ActionGroupDelete, audit.ResourceGroup, dn, nil) {
		return
	}

	if err := s.ldapClient.DeleteGroup(dn); err != nil {
		writeServerError(w, r, "delete group", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "group deleted"})
}

type memberRequest struct {
	MemberUID string `json:"memberUid"`
}

func (s *Server) handleAddGroupMember(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.GroupBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	var req memberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.MemberUID == "" {
		writeError(w, http.StatusBadRequest, "memberUid is required")
		return
	}

	if !s.auditMutating(w, r, audit.ActionMemberAdd, audit.ResourceGroup, dn,
		map[string]interface{}{"memberUid": req.MemberUID}) {
		return
	}

	if err := s.ldapClient.AddGroupMember(dn, req.MemberUID); err != nil {
		writeServerError(w, r, "add member", err)
		return
	}

	s.invalidateIfAdminGroupChange(r, dn, req.MemberUID)

	writeJSON(w, http.StatusOK, map[string]string{"message": "member added"})
}

func (s *Server) handleRemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.GroupBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	var req memberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.MemberUID == "" {
		writeError(w, http.StatusBadRequest, "memberUid is required")
		return
	}

	if !s.auditMutating(w, r, audit.ActionMemberRemove, audit.ResourceGroup, dn,
		map[string]interface{}{"memberUid": req.MemberUID}) {
		return
	}

	if err := s.ldapClient.RemoveGroupMember(dn, req.MemberUID); err != nil {
		writeServerError(w, r, "remove member", err)
		return
	}

	s.invalidateIfAdminGroupChange(r, dn, req.MemberUID)

	writeJSON(w, http.StatusOK, map[string]string{"message": "member removed"})
}

func (s *Server) handleUpdateGroupSamba(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.GroupBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	var req ldap.UpdateSambaGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.auditMutating(w, r, audit.ActionGroupUpdate, audit.ResourceGroup, dn,
		map[string]interface{}{"action": "samba_update"}) {
		return
	}

	group, err := s.ldapClient.SetSambaGroupAttributes(dn, req)
	if err != nil {
		writeServerError(w, r, "update samba group attributes", err)
		return
	}

	writeJSON(w, http.StatusOK, group)
}

// groupCreateFields produces the "field: value" dump for the group creation
// notification. Only non-empty attributes are listed.
func groupCreateFields(req ldap.CreateGroupRequest) []audit.FieldChange {
	var fields []audit.FieldChange
	add := func(label, value string) {
		if value != "" {
			fields = append(fields, audit.FieldChange{Field: label, New: value})
		}
	}

	add("Name", req.CN)
	if req.GIDNumber != 0 {
		add("GID number", strconv.Itoa(req.GIDNumber))
	}
	add("Description", req.Description)
	if len(req.MemberUIDs) > 0 {
		add("Members", strings.Join(req.MemberUIDs, ", "))
	}

	return fields
}

// groupDisplayName picks the most human-friendly label for a group.
func groupDisplayName(g *ldap.Group) string {
	if g.DisplayName != "" {
		return g.DisplayName
	}
	return g.CN
}

// groupUpdateChanges diffs a group update against its pre-update state. The
// description is a plain old/new change; membership changes are emitted as one
// entry per added or removed member. Members are only diffed when the request
// actually carries a member list (non-nil slice).
func groupUpdateChanges(before *ldap.Group, req ldap.UpdateGroupRequest) []audit.FieldChange {
	var changes []audit.FieldChange

	if req.Description != nil && *req.Description != before.Description {
		changes = append(changes, audit.FieldChange{Field: "Description", Old: before.Description, New: *req.Description})
	}

	if req.MemberUIDs != nil {
		oldSet := make(map[string]struct{}, len(before.MemberUIDs))
		for _, uid := range before.MemberUIDs {
			oldSet[uid] = struct{}{}
		}
		newSet := make(map[string]struct{}, len(req.MemberUIDs))
		for _, uid := range req.MemberUIDs {
			newSet[uid] = struct{}{}
		}
		for _, uid := range req.MemberUIDs {
			if _, ok := oldSet[uid]; !ok {
				changes = append(changes, audit.FieldChange{Field: "Member added", New: uid})
			}
		}
		for _, uid := range before.MemberUIDs {
			if _, ok := newSet[uid]; !ok {
				changes = append(changes, audit.FieldChange{Field: "Member removed", Old: uid})
			}
		}
	}

	return changes
}
