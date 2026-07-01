package api

import (
	"encoding/json"
	"net/http"

	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/auth"
)

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req auth.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	resp, err := s.authService.Login(r.Context(), req)
	if err != nil {
		// Log the attempt with the username the client supplied. The DN is
		// unknown (auth failed before we could resolve it) so we leave it
		// blank; the recorded actor_uid lets operators correlate brute-force
		// attempts with the rate-limit middleware's 429s.
		_ = s.auditLogger.LogWithActor(r.Context(), "", req.Username,
			audit.ActionLoginFailed, audit.ResourceUser, "", nil)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	_ = s.auditLogger.LogWithActor(r.Context(), resp.Session.UserDN, resp.Session.UserUID,
		audit.ActionLogin, audit.ResourceUser, resp.Session.UserDN, nil)

	s.setSessionCookie(w, resp.Token)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	token := tokenFromRequest(r)
	if token == "" {
		writeError(w, http.StatusBadRequest, "missing session token")
		return
	}
	session := auth.GetSessionFromContext(r.Context())

	if err := s.authService.Logout(r.Context(), token); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to logout")
		return
	}

	if session != nil {
		_ = s.auditLogger.LogWithActor(r.Context(), session.UserDN, session.UserUID,
			audit.ActionLogout, audit.ResourceUser, session.UserDN, nil)
	}

	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	session := auth.GetSessionFromContext(r.Context())
	if session == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	writeJSON(w, http.StatusOK, session)
}

// handleGetMyAvatar returns the current user's jpegPhoto (base64) for the
// header avatar. The photo is deliberately not carried in the session blob —
// it can weigh megabytes and would be deserialised on every authenticated
// request — so it is fetched from LDAP on demand here and cached by the client.
// A missing photo (or a failed lookup) yields an empty value; the header avatar
// falls back to the user's initials.
func (s *Server) handleGetMyAvatar(w http.ResponseWriter, r *http.Request) {
	session := auth.GetSessionFromContext(r.Context())
	if session == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var photo string
	if user, err := s.ldapClient.GetUser(session.UserDN); err == nil && user != nil {
		photo = user.JpegPhoto
	}
	writeJSON(w, http.StatusOK, map[string]string{"jpegPhoto": photo})
}

func (s *Server) handleChangeMyPassword(w http.ResponseWriter, r *http.Request) {
	session := auth.GetSessionFromContext(r.Context())
	if session == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
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

	if !s.auditMutating(w, r, audit.ActionUserUpdate, audit.ResourceUser, session.UserDN,
		map[string]interface{}{"action": "self_password_change"}) {
		return
	}

	if err := s.ldapClient.ChangePassword(session.UserDN, req.Password); err != nil {
		writeServerError(w, r, "change password", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "password changed"})
}
