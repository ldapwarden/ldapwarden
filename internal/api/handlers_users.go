package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// handleListUsers returns one page of a light user projection. Sorting,
// paging and the active/inactive filter are applied here so the browser
// receives a page instead of the whole directory (see ldap.SearchUsersPage).
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	opts := ldap.UserListOptions{ListOptions: parseListOptions(r)}
	switch ldap.UserStatus(r.URL.Query().Get("status")) {
	case ldap.UserStatusActive:
		opts.Status = ldap.UserStatusActive
	case ldap.UserStatusInactive:
		opts.Status = ldap.UserStatusInactive
	}

	result, err := s.ldapClient.SearchUsersPage(opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":          result.Items,
		"total":         result.Total,
		"inactiveCount": result.InactiveCount,
		"limit":         opts.Limit,
		"offset":        opts.Offset,
		"truncated":     result.Truncated,
	})
}

// handleGetUserPhoto serves a user's jpegPhoto as an image. Photos are excluded
// from the list projection; rows that have one point an <img> here, which lets
// the browser cache them and fetch only what is on screen.
func (s *Server) handleGetUserPhoto(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.UserBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	photo, err := s.ldapClient.GetUserPhoto(dn)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if len(photo) == 0 {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	// Private: the photo is only readable by an authenticated session, so it
	// must not be stored by shared caches. must-revalidate keeps a replaced
	// photo from sticking around after the max-age window.
	w.Header().Set("Cache-Control", "private, max-age=300, must-revalidate")
	w.Header().Set("Content-Length", strconv.Itoa(len(photo)))
	// The bytes come from the directory, so gosec's taint analysis flags the
	// write. They are served as image/jpeg, and secureHeadersMiddleware sets
	// X-Content-Type-Options: nosniff on every response, so the browser will not
	// reinterpret them as markup.
	if _, err := w.Write(photo); err != nil { //nolint:gosec // typed image response, nosniff enforced
		return
	}
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

	// Zero means "allocate one": the client had no opinion, so the number is
	// settled server-side at write time rather than read from a form that loaded
	// minutes ago. A number that *is* given still has to be in range, and
	// CreateUser refuses it if another account holds it.
	if req.UIDNumber != 0 {
		if err := validatePOSIXID("uidNumber", req.UIDNumber, s.ldapClient.MinUID()); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	if req.GIDNumber != 0 {
		if err := validatePOSIXID("gidNumber", req.GIDNumber, s.ldapClient.MinGID()); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
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

// handleChangeUserUID renames a user (uid is the RDN of the DN) and migrates
// the references that pointed at the old uid/DN. It is a deliberately separate,
// warned action rather than part of the normal update flow. Responds with the
// renamed user and any non-fatal migration warnings.
func (s *Server) handleChangeUserUID(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.UserBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	if s.refuseSelfTarget(w, r, dn, "rename") {
		return
	}

	var req struct {
		NewUID string `json:"newUid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateRDNValue("uid", req.NewUID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	before, err := s.ldapClient.GetUser(dn)
	if err != nil || before == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if req.NewUID == before.UID {
		writeError(w, http.StatusBadRequest, "the new username is the same as the current one")
		return
	}
	if existing, _ := s.ldapClient.GetUserByUID(req.NewUID); existing != nil {
		writeError(w, http.StatusConflict, "a user with this username already exists")
		return
	}

	details := map[string]interface{}{
		audit.DetailsKeyResourceName: userDisplayName(before),
		audit.DetailsKeyChanges: []audit.FieldChange{
			{Field: "Username", Old: before.UID, New: req.NewUID},
		},
	}
	if !s.auditMutating(w, r, audit.ActionUserRename, audit.ResourceUser, dn, details) {
		return
	}

	newDN, warnings, err := s.ldapClient.RenameUser(dn, before.UID, req.NewUID)
	if err != nil {
		writeServerError(w, r, "rename user", err)
		return
	}

	// The renamed user's active sessions are keyed by the old DN — force
	// re-login.
	s.invalidateSessions(r, dn, "uid changed")

	user, err := s.ldapClient.GetUser(newDN)
	if err != nil {
		writeServerError(w, r, "read renamed user", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user":     user,
		"warnings": warnings,
	})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.UserBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	if s.refuseSelfTarget(w, r, dn, "delete") {
		return
	}

	// An entry that is not there is a 404, as it is on every other user route —
	// the delete path used to surface the directory's "No Such Object" as a 500.
	if user, err := s.ldapClient.GetUser(dn); err != nil || user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	if !s.auditMutating(w, r, audit.ActionUserDelete, audit.ResourceUser, dn, s.userNameDetails(dn)) {
		return
	}

	// warnings names the references the delete could not clean up (group
	// memberships, sudo grants, manager attributes). The entry itself is gone
	// either way, so they are reported rather than turned into a failure.
	warnings, err := s.ldapClient.DeleteUser(dn)
	if err != nil {
		writeServerError(w, r, "delete user", err)
		return
	}

	s.invalidateSessions(r, dn, "user deleted")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "user deleted",
		"warnings": warnings,
	})
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

	if s.refuseSelfTarget(w, r, dn, "lock") {
		return
	}

	if !s.auditMutating(w, r, audit.ActionUserLock, audit.ResourceUser, dn, s.userNameDetails(dn)) {
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

	if !s.auditMutating(w, r, audit.ActionUserUnlock, audit.ResourceUser, dn, s.userNameDetails(dn)) {
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
	// Setting an expiry on your own account still locks you out — later, but with
	// no way back once it lands. Clearing one cannot, and is how you undo it.
	if req.ExpirationDate != "" && s.refuseSelfTarget(w, r, dn, "set an expiration date on") {
		return
	}

	if req.ExpirationDate == "" {
		action = "expiration_cleared"
		message = "expiration date cleared"
	}

	if !s.auditMutating(w, r, audit.ActionUserUpdate, audit.ResourceUser, dn,
		s.userActionDetails(dn, map[string]interface{}{"action": action, "expirationDate": req.ExpirationDate})) {
		return
	}

	if err := s.ldapClient.SetUserExpiration(dn, req.ExpirationDate); err != nil {
		if errors.Is(err, ldap.ErrScheduledExpiryUnsupported) {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
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
		s.userActionDetails(dn, map[string]interface{}{"action": "password_change"})) {
		return
	}

	if err := s.ldapClient.ChangePassword(dn, req.Password); err != nil {
		s.writePasswordError(w, r, "change password", dn, err)
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

	if s.refuseSelfTarget(w, r, dn, "remove the password of") {
		return
	}

	if !s.auditMutating(w, r, audit.ActionUserUpdate, audit.ResourceUser, dn,
		s.userActionDetails(dn, map[string]interface{}{"action": "password_remove"})) {
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
		s.userActionDetails(dn, map[string]interface{}{"action": "ssh_keys_update", "keyCount": len(req.Keys)})) {
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
		s.userActionDetails(dn, map[string]interface{}{"action": "ssh_key_add"})) {
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
		s.userActionDetails(dn, map[string]interface{}{"action": "ssh_key_remove"})) {
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
		s.userActionDetails(dn, map[string]interface{}{"action": "samba_update"})) {
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

	// shadowExpire no longer drives this app's account status — slapd ignores it —
	// but PAM/SSSD on the hosts do read it, so expiring your own entry there is
	// still a way to lock yourself out of your machines. Only an actual expiry is
	// refused: 0 and the "never" ceiling clear it.
	if req.ShadowExpire != nil && *req.ShadowExpire > 0 && *req.ShadowExpire < ldap.ShadowExpireNever &&
		s.refuseSelfTarget(w, r, dn, "set a shadow expiry on") {
		return
	}

	if !s.auditMutating(w, r, audit.ActionUserUpdate, audit.ResourceUser, dn,
		s.userActionDetails(dn, map[string]interface{}{"action": "shadow_update"})) {
		return
	}

	user, err := s.ldapClient.SetShadowUserAttributes(dn, req)
	if err != nil {
		writeServerError(w, r, "update shadow attributes", err)
		return
	}

	writeJSON(w, http.StatusOK, user)
}

// userActionDetails builds the audit details map for a user sub-action
// (password reset, SSH keys, Samba, shadow, …) that records a generic
// user.update. It attaches a best-effort friendly resource name so the
// notification reads "Alexandre Duffez (aduffez)" rather than the bare uid,
// then merges the action-specific fields. A failed lookup simply omits the
// name and the mailer falls back to the RDN.
func (s *Server) userActionDetails(dn string, extra map[string]interface{}) map[string]interface{} {
	if before, err := s.ldapClient.GetUser(dn); err == nil && before != nil {
		extra[audit.DetailsKeyResourceName] = userDisplayName(before)
	}
	return extra
}

// userDisplayName picks the most human-friendly label available for a user,
// used for audit subjects and notification headers. When a friendly name
// exists it is qualified with the uid ("Romain Besson (rbesson)") so the email
// carries both the readable name and the stable identifier; with no friendly
// name it falls back to the bare uid.
func userDisplayName(u *ldap.User) string {
	return labelWithID(userFriendlyName(u.DisplayName, u.GivenName, u.SN, u.CN), u.UID)
}

// createUserDisplayName picks the friendliest label from a creation request.
func createUserDisplayName(req ldap.CreateUserRequest) string {
	return labelWithID(userFriendlyName(req.DisplayName, req.GivenName, req.SN, req.CN), req.UID)
}

// userNameDetails does a best-effort lookup of a user so an audit entry that
// carries no field diff (lock/unlock/delete) can still name the resource as
// "displayName (uid)" in the notification. The lookup happens before the
// mutation, so the entry still exists. A failed read yields nil — the mutation
// and its audit row must never depend on it.
func (s *Server) userNameDetails(dn string) map[string]interface{} {
	if before, err := s.ldapClient.GetUser(dn); err == nil && before != nil {
		return map[string]interface{}{audit.DetailsKeyResourceName: userDisplayName(before)}
	}
	return nil
}

// userFriendlyName returns the best human-readable name for a user from the
// available name attributes, or "" when none is set.
func userFriendlyName(displayName, givenName, sn, cn string) string {
	switch {
	case displayName != "":
		return displayName
	case givenName != "" || sn != "":
		return strings.TrimSpace(givenName + " " + sn)
	case cn != "":
		return cn
	default:
		return ""
	}
}

// labelWithID renders a resource label as "friendly (id)" when a friendly name
// is present and distinct from the identifier, otherwise just the identifier.
// It is the shared shape for audit subjects: "displayName (uid)" for users,
// "description (cn)" for groups.
func labelWithID(friendly, id string) string {
	if friendly == "" || friendly == id {
		return id
	}
	return fmt.Sprintf("%s (%s)", friendly, id)
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

// handleChangePosixIDs writes a user's uidNumber and gidNumber.
//
// Its own route, like change-uid, and for a stronger reason than the rename: the
// rename migrates every reference it can find, while this one cannot follow the
// numbers where they actually matter. A uidNumber is what the filesystem stores
// in every inode the account owns, on every host, and nothing in the directory
// records where those are. The endpoint's job is therefore to make the change
// safely and then say plainly what it did not do.
func (s *Server) handleChangePosixIDs(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.UserBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	var req struct {
		UIDNumber *int `json:"uidNumber"`
		GIDNumber *int `json:"gidNumber"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.UIDNumber == nil && req.GIDNumber == nil {
		writeError(w, http.StatusBadRequest, "no uidNumber or gidNumber given")
		return
	}

	before, err := s.ldapClient.GetUser(dn)
	if err != nil || before == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	if req.UIDNumber != nil {
		if err := validatePOSIXID("uidNumber", *req.UIDNumber, s.ldapClient.MinUID()); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		// Two accounts on one uidNumber are one user to the kernel: same files,
		// same access, and no way to tell them apart afterwards.
		if holder, herr := s.ldapClient.UserByUIDNumber(*req.UIDNumber); herr == nil &&
			holder != nil && holder.DN != dn {
			writeError(w, http.StatusConflict, fmt.Sprintf(
				"uid number %d already belongs to %q. Two accounts sharing one is the same "+
					"user as far as the system is concerned.", *req.UIDNumber, holder.UID))
			return
		}
	}
	if req.GIDNumber != nil {
		if err := validatePOSIXID("gidNumber", *req.GIDNumber, s.ldapClient.MinGID()); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	changes := []audit.FieldChange{}
	if req.UIDNumber != nil && *req.UIDNumber != before.UIDNumber {
		changes = append(changes, audit.FieldChange{
			Field: "UID number",
			Old:   strconv.Itoa(before.UIDNumber),
			New:   strconv.Itoa(*req.UIDNumber),
		})
	}
	if req.GIDNumber != nil && *req.GIDNumber != before.GIDNumber {
		changes = append(changes, audit.FieldChange{
			Field: "GID number",
			Old:   strconv.Itoa(before.GIDNumber),
			New:   strconv.Itoa(*req.GIDNumber),
		})
	}
	if len(changes) == 0 {
		writeError(w, http.StatusBadRequest, "the numbers given are the ones already set")
		return
	}

	details := map[string]interface{}{
		audit.DetailsKeyResourceName: userDisplayName(before),
		audit.DetailsKeyChanges:      changes,
	}
	if !s.auditMutating(w, r, audit.ActionUserUpdate, audit.ResourceUser, dn, details) {
		return
	}

	warnings, err := s.ldapClient.ChangePosixIDs(dn, req.UIDNumber, req.GIDNumber)
	if err != nil {
		writeServerError(w, r, "change posix ids", err)
		return
	}

	user, err := s.ldapClient.GetUser(dn)
	if err != nil {
		writeServerError(w, r, "read user", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user":     user,
		"warnings": warnings,
	})
}
