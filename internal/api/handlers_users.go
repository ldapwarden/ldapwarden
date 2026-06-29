package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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
	details := map[string]interface{}{
		audit.DetailsKeyResourceName: createUserDisplayName(req),
		audit.DetailsKeyChanges:      userCreateFields(req),
	}
	if !s.auditMutating(w, r, audit.ActionUserCreate, audit.ResourceUser, plannedDN, details) {
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

	// Capture the pre-update state so the audit log (and notification email)
	// can record a field-level diff. Best-effort: a failed read just yields no
	// diff and must not block the mutation.
	var details map[string]interface{}
	if before, err := s.ldapClient.GetUser(dn); err == nil && before != nil {
		details = map[string]interface{}{audit.DetailsKeyResourceName: userDisplayName(before)}
		if changes := userUpdateChanges(before, req); len(changes) > 0 {
			details[audit.DetailsKeyChanges] = changes
		}
	}

	if !s.auditMutating(w, r, audit.ActionUserUpdate, audit.ResourceUser, dn, details) {
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

	s.invalidateSessions(r, dn, "user deleted")

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

	s.invalidateSessions(r, dn, "user locked")

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

	s.invalidateSessions(r, dn, "password changed")

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

	s.invalidateSessions(r, dn, "password removed")

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

// validateShadowRequest rejects nonsensical shadow integers. -1 is the
// documented "delete this attribute" sentinel; any other negative value is
// invalid for a shadow day-count field, so we reject it rather than writing it
// to LDAP. Zero and positive values are passed through unchanged.
func validateShadowRequest(req ldap.UpdateShadowUserRequest) error {
	fields := []struct {
		name string
		val  *int
	}{
		{"shadowLastChange", req.ShadowLastChange},
		{"shadowMin", req.ShadowMin},
		{"shadowMax", req.ShadowMax},
		{"shadowWarning", req.ShadowWarning},
		{"shadowInactive", req.ShadowInactive},
		{"shadowExpire", req.ShadowExpire},
		{"shadowFlag", req.ShadowFlag},
	}
	for _, f := range fields {
		if f.val != nil && *f.val < -1 {
			return fmt.Errorf("%s must be >= -1 (-1 deletes the attribute)", f.name)
		}
	}
	return nil
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

	if err := validateShadowRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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

// userDisplayName picks the most human-friendly label available for a user,
// used for audit subjects and notification headers.
func userDisplayName(u *ldap.User) string {
	switch {
	case u.DisplayName != "":
		return u.DisplayName
	case u.GivenName != "" || u.SN != "":
		return strings.TrimSpace(u.GivenName + " " + u.SN)
	case u.CN != "":
		return u.CN
	default:
		return u.UID
	}
}

// createUserDisplayName picks the friendliest label from a creation request.
func createUserDisplayName(req ldap.CreateUserRequest) string {
	switch {
	case req.DisplayName != "":
		return req.DisplayName
	case req.GivenName != "" || req.SN != "":
		return strings.TrimSpace(req.GivenName + " " + req.SN)
	case req.CN != "":
		return req.CN
	default:
		return req.UID
	}
}

// userCreateFields produces the "field: value" dump shown in the creation
// notification. Only non-empty attributes are listed; the password is masked.
func userCreateFields(req ldap.CreateUserRequest) []audit.FieldChange {
	var fields []audit.FieldChange
	add := func(label, value string) {
		if value != "" {
			fields = append(fields, audit.FieldChange{Field: label, New: value})
		}
	}

	add("Username", req.UID)
	add("Email", req.Mail)
	add("Title", req.Title)
	add("Department", req.Department)
	add("Organization", req.Organization)
	add("Employee number", req.EmployeeNumber)
	add("Employee type", req.EmployeeType)
	add("Manager", req.Manager)
	add("Login shell", req.LoginShell)
	add("Home directory", req.HomeDirectory)
	add("Expiration date", req.ExpirationDate)
	if len(req.Groups) > 0 {
		add("Groups", strings.Join(req.Groups, ", "))
	}
	if req.Password != "" {
		fields = append(fields, audit.FieldChange{Field: "Password", Masked: true})
	}

	return fields
}

// userUpdateChanges diffs the requested update against the pre-update user,
// returning one FieldChange per attribute that actually changes. Only fields
// present in the request (non-nil pointers) are considered. Password and photo
// are masked: the change is recorded but the (secret or bulky) value is never
// emitted.
func userUpdateChanges(before *ldap.User, req ldap.UpdateUserRequest) []audit.FieldChange {
	var changes []audit.FieldChange

	str := func(label string, newVal *string, oldVal string) {
		if newVal == nil || *newVal == oldVal {
			return
		}
		changes = append(changes, audit.FieldChange{Field: label, Old: oldVal, New: *newVal})
	}

	str("First name", req.GivenName, before.GivenName)
	str("Last name", req.SN, before.SN)
	str("Common name", req.CN, before.CN)
	str("Display name", req.DisplayName, before.DisplayName)
	str("Email", req.Mail, before.Mail)
	str("Phone", req.TelephoneNumber, before.TelephoneNumber)
	str("Title", req.Title, before.Title)
	str("Department", req.Department, before.Department)
	str("Organization", req.Organization, before.Organization)
	str("Employee number", req.EmployeeNumber, before.EmployeeNumber)
	str("Employee type", req.EmployeeType, before.EmployeeType)
	str("Initials", req.Initials, before.Initials)
	str("Manager", req.Manager, before.Manager)
	str("Home directory", req.HomeDirectory, before.HomeDirectory)
	str("Login shell", req.LoginShell, before.LoginShell)
	str("GECOS", req.Gecos, before.Gecos)
	str("Description", req.Description, before.Description)
	str("Password policy", req.PwdPolicySubentry, before.PwdPolicySubentry)

	if req.Password != nil {
		changes = append(changes, audit.FieldChange{Field: "Password", Masked: true})
	}
	if req.JpegPhoto != nil && *req.JpegPhoto != before.JpegPhoto {
		changes = append(changes, audit.FieldChange{Field: "Photo", Masked: true})
	}

	return changes
}
