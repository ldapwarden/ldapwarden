package api

import (
	"net/http"

	"github.com/ldapwarden/ldapwarden/internal/auth"
)

// refuseSelfTarget blocks the actions that would cut the caller off from the
// application when aimed at their own account: locking it, deleting it,
// clearing its password, or renaming its uid.
//
// The permission model cannot express this — an admin legitimately holds
// users:write and users:delete over every entry, including their own — so the
// guard has to be per-action. Without it a single click locks the operator out,
// and if they are the only admin the way back is ldapmodify on the directory.
//
// It lives in the API rather than only in the UI because hiding a button is not
// a control: the endpoints are reachable directly.
//
// Returns true when the request was refused (a response has been written).
func (s *Server) refuseSelfTarget(w http.ResponseWriter, r *http.Request, dn, action string) bool {
	session := auth.GetSessionFromContext(r.Context())
	if session == nil || !sameDN(session.UserDN, dn) {
		return false
	}

	// 409: the request is well-formed and permitted in principle, but conflicts
	// with the state of the caller's own session.
	writeError(w, http.StatusConflict,
		"you cannot "+action+" the account you are signed in with — ask another administrator, or use a different account")
	return true
}
