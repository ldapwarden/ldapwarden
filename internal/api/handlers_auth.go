package api

import (
	"encoding/json"
	"net/http"
	"strings"

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

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 {
		writeError(w, http.StatusBadRequest, "invalid authorization header")
		return
	}

	token := parts[1]
	session := auth.GetSessionFromContext(r.Context())

	if err := s.authService.Logout(r.Context(), token); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to logout")
		return
	}

	if session != nil {
		_ = s.auditLogger.LogWithActor(r.Context(), session.UserDN, session.UserUID,
			audit.ActionLogout, audit.ResourceUser, session.UserDN, nil)
	}

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

	if err := s.ldapClient.ChangePassword(session.UserDN, req.Password); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to change password: "+err.Error())
		return
	}

	_ = s.auditLogger.Log(r.Context(), audit.ActionUserUpdate, audit.ResourceUser, session.UserDN,
		map[string]interface{}{"action": "self_password_change"})

	writeJSON(w, http.StatusOK, map[string]string{"message": "password changed"})
}
