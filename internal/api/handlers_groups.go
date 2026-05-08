package api

import (
	"encoding/json"
	"net/http"
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
	_ = s.authService.InvalidateUserSessions(r.Context(), user.DN)
}

func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.ldapClient.ListGroups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list groups")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  groups,
		"total": len(groups),
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

	if req.GIDNumber == 0 {
		writeError(w, http.StatusBadRequest, "gidNumber is required")
		return
	}

	group, err := s.ldapClient.CreateGroup(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create group: "+err.Error())
		return
	}

	_ = s.auditLogger.Log(r.Context(), audit.ActionGroupCreate, audit.ResourceGroup, group.DN,
		map[string]interface{}{"cn": group.CN})

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

	group, err := s.ldapClient.UpdateGroup(dn, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update group: "+err.Error())
		return
	}

	_ = s.auditLogger.Log(r.Context(), audit.ActionGroupUpdate, audit.ResourceGroup, group.DN, nil)

	writeJSON(w, http.StatusOK, group)
}

func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.GroupBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	if err := s.ldapClient.DeleteGroup(dn); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete group: "+err.Error())
		return
	}

	_ = s.auditLogger.Log(r.Context(), audit.ActionGroupDelete, audit.ResourceGroup, dn, nil)

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

	if err := s.ldapClient.AddGroupMember(dn, req.MemberUID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add member: "+err.Error())
		return
	}

	s.invalidateIfAdminGroupChange(r, dn, req.MemberUID)
	_ = s.auditLogger.Log(r.Context(), audit.ActionMemberAdd, audit.ResourceGroup, dn,
		map[string]interface{}{"memberUid": req.MemberUID})

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

	if err := s.ldapClient.RemoveGroupMember(dn, req.MemberUID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove member: "+err.Error())
		return
	}

	s.invalidateIfAdminGroupChange(r, dn, req.MemberUID)
	_ = s.auditLogger.Log(r.Context(), audit.ActionMemberRemove, audit.ResourceGroup, dn,
		map[string]interface{}{"memberUid": req.MemberUID})

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

	group, err := s.ldapClient.SetSambaGroupAttributes(dn, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	_ = s.auditLogger.Log(r.Context(), audit.ActionGroupUpdate, audit.ResourceGroup, dn,
		map[string]interface{}{"action": "samba_update"})

	writeJSON(w, http.StatusOK, group)
}
