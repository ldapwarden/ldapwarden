package api

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.ldapClient.ListUsers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  users,
		"total": len(users),
	})
}

func (s *Server) handleGetUser(w http.ResponseWriter, r *http.Request) {
	dnEncoded := chi.URLParam(r, "dn")
	dn, err := url.QueryUnescape(dnEncoded)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid DN encoding")
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

	if req.UID == "" || req.SN == "" || req.GivenName == "" {
		writeError(w, http.StatusBadRequest, "uid, sn, and givenName are required")
		return
	}

	if req.UIDNumber == 0 {
		writeError(w, http.StatusBadRequest, "uidNumber is required")
		return
	}

	if req.GIDNumber == 0 {
		writeError(w, http.StatusBadRequest, "gidNumber is required")
		return
	}

	user, err := s.ldapClient.CreateUser(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create user: "+err.Error())
		return
	}

	s.auditLogger.Log(r.Context(), audit.ActionUserCreate, audit.ResourceUser, user.DN,
		map[string]interface{}{"uid": user.UID})

	writeJSON(w, http.StatusCreated, user)
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	dnEncoded := chi.URLParam(r, "dn")
	dn, err := url.QueryUnescape(dnEncoded)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid DN encoding")
		return
	}

	var req ldap.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := s.ldapClient.UpdateUser(dn, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update user: "+err.Error())
		return
	}

	s.auditLogger.Log(r.Context(), audit.ActionUserUpdate, audit.ResourceUser, user.DN, nil)

	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	dnEncoded := chi.URLParam(r, "dn")
	dn, err := url.QueryUnescape(dnEncoded)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid DN encoding")
		return
	}

	if err := s.ldapClient.DeleteUser(dn); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete user: "+err.Error())
		return
	}

	s.auditLogger.Log(r.Context(), audit.ActionUserDelete, audit.ResourceUser, dn, nil)

	writeJSON(w, http.StatusOK, map[string]string{"message": "user deleted"})
}

func (s *Server) handleGetUserGroups(w http.ResponseWriter, r *http.Request) {
	dnEncoded := chi.URLParam(r, "dn")
	dn, err := url.QueryUnescape(dnEncoded)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid DN encoding")
		return
	}

	user, err := s.ldapClient.GetUser(dn)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	groups, err := s.ldapClient.GetUserGroups(user.UID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get user groups: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  groups,
		"total": len(groups),
	})
}

func (s *Server) handleLockUser(w http.ResponseWriter, r *http.Request) {
	dnEncoded := chi.URLParam(r, "dn")
	dn, err := url.QueryUnescape(dnEncoded)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid DN encoding")
		return
	}

	if err := s.ldapClient.LockUser(dn); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lock user: "+err.Error())
		return
	}

	s.auditLogger.Log(r.Context(), audit.ActionUserLock, audit.ResourceUser, dn, nil)

	writeJSON(w, http.StatusOK, map[string]string{"message": "user locked"})
}

func (s *Server) handleUnlockUser(w http.ResponseWriter, r *http.Request) {
	dnEncoded := chi.URLParam(r, "dn")
	dn, err := url.QueryUnescape(dnEncoded)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid DN encoding")
		return
	}

	if err := s.ldapClient.UnlockUser(dn); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to unlock user: "+err.Error())
		return
	}

	s.auditLogger.Log(r.Context(), audit.ActionUserUnlock, audit.ResourceUser, dn, nil)

	writeJSON(w, http.StatusOK, map[string]string{"message": "user unlocked"})
}

func (s *Server) handleSetUserExpiration(w http.ResponseWriter, r *http.Request) {
	dnEncoded := chi.URLParam(r, "dn")
	dn, err := url.QueryUnescape(dnEncoded)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid DN encoding")
		return
	}

	var req struct {
		ExpirationDate string `json:"expirationDate"` // ISO date format (YYYY-MM-DD) or empty to clear
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.ldapClient.SetUserExpiration(dn, req.ExpirationDate); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set expiration: "+err.Error())
		return
	}

	action := "expiration_set"
	message := "expiration date set"
	if req.ExpirationDate == "" {
		action = "expiration_cleared"
		message = "expiration date cleared"
	}

	s.auditLogger.Log(r.Context(), audit.ActionUserUpdate, audit.ResourceUser, dn,
		map[string]interface{}{"action": action, "expirationDate": req.ExpirationDate})

	writeJSON(w, http.StatusOK, map[string]string{"message": message})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	dnEncoded := chi.URLParam(r, "dn")
	dn, err := url.QueryUnescape(dnEncoded)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid DN encoding")
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

	if err := s.ldapClient.ChangePassword(dn, req.Password); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to change password: "+err.Error())
		return
	}

	s.auditLogger.Log(r.Context(), audit.ActionUserUpdate, audit.ResourceUser, dn,
		map[string]interface{}{"action": "password_change"})

	writeJSON(w, http.StatusOK, map[string]string{"message": "password changed"})
}

func (s *Server) handleSetSSHKeys(w http.ResponseWriter, r *http.Request) {
	dnEncoded := chi.URLParam(r, "dn")
	dn, err := url.QueryUnescape(dnEncoded)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid DN encoding")
		return
	}

	var req struct {
		Keys []string `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.ldapClient.SetSSHPublicKeys(dn, req.Keys); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set SSH keys: "+err.Error())
		return
	}

	s.auditLogger.Log(r.Context(), audit.ActionUserUpdate, audit.ResourceUser, dn,
		map[string]interface{}{"action": "ssh_keys_update", "keyCount": len(req.Keys)})

	user, _ := s.ldapClient.GetUser(dn)
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleAddSSHKey(w http.ResponseWriter, r *http.Request) {
	dnEncoded := chi.URLParam(r, "dn")
	dn, err := url.QueryUnescape(dnEncoded)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid DN encoding")
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

	if err := s.ldapClient.AddSSHPublicKey(dn, req.Key); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add SSH key: "+err.Error())
		return
	}

	s.auditLogger.Log(r.Context(), audit.ActionUserUpdate, audit.ResourceUser, dn,
		map[string]interface{}{"action": "ssh_key_add"})

	user, _ := s.ldapClient.GetUser(dn)
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleRemoveSSHKey(w http.ResponseWriter, r *http.Request) {
	dnEncoded := chi.URLParam(r, "dn")
	dn, err := url.QueryUnescape(dnEncoded)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid DN encoding")
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

	if err := s.ldapClient.RemoveSSHPublicKey(dn, req.Key); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove SSH key: "+err.Error())
		return
	}

	s.auditLogger.Log(r.Context(), audit.ActionUserUpdate, audit.ResourceUser, dn,
		map[string]interface{}{"action": "ssh_key_remove"})

	user, _ := s.ldapClient.GetUser(dn)
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleUpdateUserSamba(w http.ResponseWriter, r *http.Request) {
	dnEncoded := chi.URLParam(r, "dn")
	dn, err := url.QueryUnescape(dnEncoded)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid DN encoding")
		return
	}

	var req ldap.UpdateSambaUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := s.ldapClient.SetSambaUserAttributes(dn, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.auditLogger.Log(r.Context(), audit.ActionUserUpdate, audit.ResourceUser, dn,
		map[string]interface{}{"action": "samba_update"})

	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleUpdateUserShadow(w http.ResponseWriter, r *http.Request) {
	dnEncoded := chi.URLParam(r, "dn")
	dn, err := url.QueryUnescape(dnEncoded)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid DN encoding")
		return
	}

	var req ldap.UpdateShadowUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := s.ldapClient.SetShadowUserAttributes(dn, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.auditLogger.Log(r.Context(), audit.ActionUserUpdate, audit.ResourceUser, dn,
		map[string]interface{}{"action": "shadow_update"})

	writeJSON(w, http.StatusOK, user)
}
