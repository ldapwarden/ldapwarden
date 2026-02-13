package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/auth"
	"github.com/ldapwarden/ldapwarden/internal/mail"
)

func (s *Server) handleSendPasswordReset(w http.ResponseWriter, r *http.Request) {
	session := auth.GetSessionFromContext(r.Context())
	if session == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	dn, err := url.PathUnescape(chi.URLParam(r, "dn"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid DN")
		return
	}

	// Get user from LDAP
	user, err := s.ldapClient.GetUser(dn)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	if user.Mail == "" {
		writeError(w, http.StatusBadRequest, "user has no email address")
		return
	}

	// Create password reset token
	token, err := s.passwordReset.CreateToken(r.Context(), user.DN, user.UID, user.Mail, session.UserDN)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create reset token: "+err.Error())
		return
	}

	// Build reset link
	resetLink := fmt.Sprintf("%s/reset-password/%s", s.config.App.PublicURL, token)

	// Send email
	displayName := user.DisplayName
	if displayName == "" {
		displayName = user.CN
	}
	if err := s.mailer.SendPasswordResetEmail(user.Mail, displayName, resetLink); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to send email: "+err.Error())
		return
	}

	_ = s.auditLogger.Log(r.Context(), audit.ActionUserUpdate, audit.ResourceUser, dn,
		map[string]interface{}{"action": "password_reset_sent", "email": user.Mail})

	writeJSON(w, http.StatusOK, map[string]string{"message": "password reset email sent"})
}

func (s *Server) handleGetPasswordResetInfo(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	tokenInfo, err := s.passwordReset.ValidateToken(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusNotFound, "invalid or expired token")
		return
	}

	// Get user display name from LDAP
	user, err := s.ldapClient.GetUser(tokenInfo.UserDN)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	displayName := user.DisplayName
	if displayName == "" {
		displayName = user.CN
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"uid":          tokenInfo.UserUID,
		"displayName":  displayName,
		"organization": s.config.App.Organization,
	})
}

func (s *Server) handleConfirmPasswordReset(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

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

	// Validate token
	tokenInfo, err := s.passwordReset.ValidateToken(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusNotFound, "invalid or expired token")
		return
	}

	// Change password in LDAP
	if err := s.ldapClient.ChangePassword(tokenInfo.UserDN, req.Password); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to change password: "+err.Error())
		return
	}

	// Get client IP
	clientIP := r.RemoteAddr
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		clientIP = realIP
	} else if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		clientIP = forwardedFor
	}

	// Mark token as used
	if err := s.passwordReset.MarkTokenUsed(r.Context(), tokenInfo.ID, clientIP); err != nil {
		// Log but don't fail - password was already changed
		fmt.Printf("failed to mark token as used: %v\n", err)
	}

	// Get WHOIS info for the IP
	whoisInfo := mail.GetWhoisInfo(clientIP)

	// Get user display name
	user, _ := s.ldapClient.GetUser(tokenInfo.UserDN)
	displayName := tokenInfo.UserUID
	if user != nil {
		if user.DisplayName != "" {
			displayName = user.DisplayName
		} else if user.CN != "" {
			displayName = user.CN
		}
	}

	// Get admin emails for notifications
	adminEmails := s.getAdminEmails()

	// Build recipient list: user + all admins
	recipients := []string{tokenInfo.UserEmail}
	for _, email := range adminEmails {
		if email != tokenInfo.UserEmail {
			recipients = append(recipients, email)
		}
	}

	// Send notification emails
	if err := s.mailer.SendPasswordChangedNotification(recipients, displayName, clientIP, whoisInfo); err != nil {
		// Log but don't fail
		fmt.Printf("failed to send notification email: %v\n", err)
	}

	// Audit log
	_ = s.auditLogger.LogWithActor(r.Context(), tokenInfo.UserDN, tokenInfo.UserUID,
		audit.ActionUserUpdate, audit.ResourceUser, tokenInfo.UserDN,
		map[string]interface{}{
			"action":    "password_reset_completed",
			"ip":        clientIP,
			"whoisInfo": whoisInfo,
		})

	writeJSON(w, http.StatusOK, map[string]string{"message": "password changed successfully"})
}

func (s *Server) getAdminEmails() []string {
	// Get all users in the admin group
	adminGroup := s.config.App.AdminGroup
	groups, err := s.ldapClient.ListGroups()
	if err != nil {
		return nil
	}

	var adminMembers []string
	for _, group := range groups {
		if group.CN == adminGroup {
			adminMembers = group.MemberUIDs
			break
		}
	}

	var emails []string
	for _, uid := range adminMembers {
		users, err := s.ldapClient.ListUsers()
		if err != nil {
			continue
		}
		for _, user := range users {
			if user.UID == uid && user.Mail != "" {
				emails = append(emails, user.Mail)
				break
			}
		}
	}

	return emails
}
