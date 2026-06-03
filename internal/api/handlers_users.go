package api

import (
	"encoding/json"
	"net/http"

	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, truncated, err := s.ldapClient.SearchUsers(r.URL.Query().Get("search"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":      users,
		"total":     len(users),
		"truncated": truncated,
	})
}

func (s *Server) handleGetUser(w http.ResponseWriter, r *http.Request) {
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

	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req ldap.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.SN == "" || req.GivenName == "" {
		writeError(w, http.StatusBadRequest, "sn and givenName are required")
		return
	}

	if err := validateRDNValue("uid", req.UID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	for _, groupCN := range req.Groups {
		if err := validateRDNValue("group cn", groupCN); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	if err := validatePOSIXID("uidNumber", req.UIDNumber, s.ldapClient.MinUID()); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := validatePOSIXID("gidNumber", req.GIDNumber, s.ldapClient.MinGID()); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// UID is validated above; UserBaseDN comes from config. The DN matches
	// what CreateUser builds internally (ldap.EscapeDN is a no-op on our
	// validated charset) so the audit row references the same resource.
	plannedDN := "uid=" + req.UID + "," + s.ldapClient.UserBaseDN()
	if !s.auditMutating(w, r, audit.ActionUserCreate, audit.ResourceUser, plannedDN,
		map[string]interface{}{"uid": req.UID}) {
		return
	}

	user, err := s.ldapClient.CreateUser(req)
	if err != nil {
		writeServerError(w, r, "create user", err)
		return
	}

	writeJSON(w, http.StatusCreated, user)
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.UserBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	var req ldap.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.auditMutating(w, r, audit.ActionUserUpdate, audit.ResourceUser, dn, nil) {
		return
	}

	user, err := s.ldapClient.UpdateUser(dn, req)
	if err != nil {
		writeServerError(w, r, "update user", err)
		return
	}

	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.UserBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	if !s.auditMutating(w, r, audit.ActionUserDelete, audit.ResourceUser, dn, nil) {
		return
	}

	if err := s.ldapClient.DeleteUser(dn); err != nil {
		writeServerError(w, r, "delete user", err)
		return
	}

	_ = s.authService.InvalidateUserSessions(r.Context(), dn)

	writeJSON(w, http.StatusOK, map[string]string{"message": "user deleted"})
}

func (s *Server) handleGetUserGroups(w http.ResponseWriter, r *http.Request) {
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

	groups, err := s.ldapClient.GetUserGroups(user.UID)
	if err != nil {
		writeServerError(w, r, "get user groups", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  groups,
		"total": len(groups),
	})
}

func (s *Server) handleLockUser(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.UserBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	if !s.auditMutating(w, r, audit.ActionUserLock, audit.ResourceUser, dn, nil) {
		return
	}

	if err := s.ldapClient.LockUser(dn); err != nil {
		writeServerError(w, r, "lock user", err)
		return
	}

	_ = s.authService.InvalidateUserSessions(r.Context(), dn)

	writeJSON(w, http.StatusOK, map[string]string{"message": "user locked"})
}

func (s *Server) handleUnlockUser(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.UserBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	if !s.auditMutating(w, r, audit.ActionUserUnlock, audit.ResourceUser, dn, nil) {
		return
	}

	if err := s.ldapClient.UnlockUser(dn); err != nil {
		writeServerError(w, r, "unlock user", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "user unlocked"})
}

func (s *Server) handleSetUserExpiration(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.UserBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	var req struct {
		ExpirationDate string `json:"expirationDate"` // ISO date format (YYYY-MM-DD) or empty to clear
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	action := "expiration_set"
	message := "expiration date set"
	if req.ExpirationDate == "" {
		action = "expiration_cleared"
		message = "expiration date cleared"
	}

	if !s.auditMutating(w, r, audit.ActionUserUpdate, audit.ResourceUser, dn,
		map[string]interface{}{"action": action, "expirationDate": req.ExpirationDate}) {
		return
	}

	if err := s.ldapClient.SetUserExpiration(dn, req.ExpirationDate); err != nil {
		writeServerError(w, r, "set expiration", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": message})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.UserBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required")
		return
	}

	if !s.auditMutating(w, r, audit.ActionUserUpdate, audit.ResourceUser, dn,
		map[string]interface{}{"action": "password_change"}) {
		return
	}

	if err := s.ldapClient.ChangePassword(dn, req.Password); err != nil {
		writeServerError(w, r, "change password", err)
		return
	}

	_ = s.authService.InvalidateUserSessions(r.Context(), dn)

	writeJSON(w, http.StatusOK, map[string]string{"message": "password changed"})
}

func (s *Server) handleRemovePassword(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.UserBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	if !s.auditMutating(w, r, audit.ActionUserUpdate, audit.ResourceUser, dn,
		map[string]interface{}{"action": "password_remove"}) {
		return
	}

	if err := s.ldapClient.RemovePassword(dn); err != nil {
		writeServerError(w, r, "remove password", err)
		return
	}

	_ = s.authService.InvalidateUserSessions(r.Context(), dn)

	writeJSON(w, http.StatusOK, map[string]string{"message": "password removed"})
}

func (s *Server) handleSetSSHKeys(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.UserBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	var req struct {
		Keys []string `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.auditMutating(w, r, audit.ActionUserUpdate, audit.ResourceUser, dn,
		map[string]interface{}{"action": "ssh_keys_update", "keyCount": len(req.Keys)}) {
		return
	}

	if err := s.ldapClient.SetSSHPublicKeys(dn, req.Keys); err != nil {
		writeServerError(w, r, "set SSH keys", err)
		return
	}

	user, _ := s.ldapClient.GetUser(dn)
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleAddSSHKey(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.UserBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	var req struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}

	if !s.auditMutating(w, r, audit.ActionUserUpdate, audit.ResourceUser, dn,
		map[string]interface{}{"action": "ssh_key_add"}) {
		return
	}

	if err := s.ldapClient.AddSSHPublicKey(dn, req.Key); err != nil {
		writeServerError(w, r, "add SSH key", err)
		return
	}

	user, _ := s.ldapClient.GetUser(dn)
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleRemoveSSHKey(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.UserBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	var req struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}

	if !s.auditMutating(w, r, audit.ActionUserUpdate, audit.ResourceUser, dn,
		map[string]interface{}{"action": "ssh_key_remove"}) {
		return
	}

	if err := s.ldapClient.RemoveSSHPublicKey(dn, req.Key); err != nil {
		writeServerError(w, r, "remove SSH key", err)
		return
	}

	user, _ := s.ldapClient.GetUser(dn)
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleUpdateUserSamba(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.UserBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	var req ldap.UpdateSambaUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.auditMutating(w, r, audit.ActionUserUpdate, audit.ResourceUser, dn,
		map[string]interface{}{"action": "samba_update"}) {
		return
	}

	user, err := s.ldapClient.SetSambaUserAttributes(dn, req)
	if err != nil {
		writeServerError(w, r, "update samba attributes", err)
		return
	}

	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleUpdateUserShadow(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.UserBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	var req ldap.UpdateShadowUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.auditMutating(w, r, audit.ActionUserUpdate, audit.ResourceUser, dn,
		map[string]interface{}{"action": "shadow_update"}) {
		return
	}

	user, err := s.ldapClient.SetShadowUserAttributes(dn, req)
	if err != nil {
		writeServerError(w, r, "update shadow attributes", err)
		return
	}

	writeJSON(w, http.StatusOK, user)
}
