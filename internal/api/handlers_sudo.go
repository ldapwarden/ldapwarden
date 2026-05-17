package api

import (
	"encoding/json"
	"net/http"

	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

func (s *Server) handleListSudoRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := s.ldapClient.ListSudoRoles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list sudo roles: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  roles,
		"total": len(roles),
	})
}

func (s *Server) handleGetSudoRole(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.SudoBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	role, err := s.ldapClient.GetSudoRole(dn)
	if err != nil {
		writeError(w, http.StatusNotFound, "sudo role not found")
		return
	}

	writeJSON(w, http.StatusOK, role)
}

func (s *Server) handleGetUserSudoRoles(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.SudoBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	user, err := s.ldapClient.GetUser(dn)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	roles, err := s.ldapClient.GetUserSudoRoles(user.UID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get user sudo roles: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  roles,
		"total": len(roles),
	})
}

func (s *Server) handleCreateSudoRole(w http.ResponseWriter, r *http.Request) {
	var req ldap.CreateSudoRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := validateRDNValue("cn", req.CN); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	plannedDN := "cn=" + req.CN + "," + s.ldapClient.SudoBaseDN()
	if !s.auditMutating(w, r, audit.ActionSudoRoleCreate, audit.ResourceSudoRole, plannedDN,
		map[string]interface{}{"cn": req.CN}) {
		return
	}

	role, err := s.ldapClient.CreateSudoRole(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create sudo role: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, role)
}

func (s *Server) handleUpdateSudoRole(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.SudoBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	var req ldap.UpdateSudoRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.auditMutating(w, r, audit.ActionSudoRoleUpdate, audit.ResourceSudoRole, dn, nil) {
		return
	}

	role, err := s.ldapClient.UpdateSudoRole(dn, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update sudo role: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, role)
}

func (s *Server) handleDeleteSudoRole(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.SudoBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	if !s.auditMutating(w, r, audit.ActionSudoRoleDelete, audit.ResourceSudoRole, dn, nil) {
		return
	}

	if err := s.ldapClient.DeleteSudoRole(dn); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete sudo role: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "sudo role deleted"})
}

func (s *Server) handleAddUserToSudoRole(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.SudoBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	var req struct {
		UID string `json:"uid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.UID == "" {
		writeError(w, http.StatusBadRequest, "uid is required")
		return
	}

	if !s.auditMutating(w, r, audit.ActionSudoRoleUserAdd, audit.ResourceSudoRole, dn,
		map[string]interface{}{"uid": req.UID}) {
		return
	}

	if err := s.ldapClient.AddUserToSudoRole(dn, req.UID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add user to sudo role: "+err.Error())
		return
	}

	role, _ := s.ldapClient.GetSudoRole(dn)
	writeJSON(w, http.StatusOK, role)
}

func (s *Server) handleRemoveUserFromSudoRole(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.SudoBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	var req struct {
		UID string `json:"uid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.UID == "" {
		writeError(w, http.StatusBadRequest, "uid is required")
		return
	}

	if !s.auditMutating(w, r, audit.ActionSudoRoleUserDel, audit.ResourceSudoRole, dn,
		map[string]interface{}{"uid": req.UID}) {
		return
	}

	if err := s.ldapClient.RemoveUserFromSudoRole(dn, req.UID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove user from sudo role: "+err.Error())
		return
	}

	role, _ := s.ldapClient.GetSudoRole(dn)
	writeJSON(w, http.StatusOK, role)
}

func (s *Server) handleGetGroupSudoRoles(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.SudoBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	group, err := s.ldapClient.GetGroup(dn)
	if err != nil {
		writeError(w, http.StatusNotFound, "group not found")
		return
	}

	roles, err := s.ldapClient.GetGroupSudoRoles(group.CN)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get group sudo roles: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  roles,
		"total": len(roles),
	})
}

func (s *Server) handleAddGroupToSudoRole(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.SudoBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	var req struct {
		CN string `json:"cn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.CN == "" {
		writeError(w, http.StatusBadRequest, "cn is required")
		return
	}

	if !s.auditMutating(w, r, audit.ActionSudoRoleGroupAdd, audit.ResourceSudoRole, dn,
		map[string]interface{}{"cn": req.CN}) {
		return
	}

	if err := s.ldapClient.AddGroupToSudoRole(dn, req.CN); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add group to sudo role: "+err.Error())
		return
	}

	role, _ := s.ldapClient.GetSudoRole(dn)
	writeJSON(w, http.StatusOK, role)
}

func (s *Server) handleRemoveGroupFromSudoRole(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.SudoBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	var req struct {
		CN string `json:"cn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.CN == "" {
		writeError(w, http.StatusBadRequest, "cn is required")
		return
	}

	if !s.auditMutating(w, r, audit.ActionSudoRoleGroupDel, audit.ResourceSudoRole, dn,
		map[string]interface{}{"cn": req.CN}) {
		return
	}

	if err := s.ldapClient.RemoveGroupFromSudoRole(dn, req.CN); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove group from sudo role: "+err.Error())
		return
	}

	role, _ := s.ldapClient.GetSudoRole(dn)
	writeJSON(w, http.StatusOK, role)
}
