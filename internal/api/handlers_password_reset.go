package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/auth"
	"github.com/ldapwarden/ldapwarden/internal/mail"
)

// Issuance caps for handleSendPasswordReset. Tunable here rather than via
// env vars because the values are policy, not deployment configuration.
const (
	// maxResetsPerUserWindow is how many resets a single user can receive
	// in any rolling resetUserWindow. Five minutes covers a typical UX
	// "didn't get the email, try again" while making inbox-flooding hard.
	maxResetsPerUserWindow = 3
	resetUserWindow        = 5 * time.Minute

	// maxResetsPerActorWindow blunts a compromised admin account: even if
	// the attacker can call the endpoint freely, they cannot fan out to
	// hundreds of users without tripping the cap.
	maxResetsPerActorWindow = 20
	resetActorWindow        = 5 * time.Minute
)

func (s *Server) handleSendPasswordReset(w http.ResponseWriter, r *http.Request) {
	session := auth.GetSessionFromContext(r.Context())
	if session == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	dn, err := resolveDN(r, s.ldapClient.UserBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
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

	// Per-actor cap: stops a compromised admin account from bulk-flooding
	// the directory with reset emails.
	actorCount, err := s.passwordReset.CountRecentByActor(r.Context(), session.UserDN, resetActorWindow)
	if err != nil {
		writeServerError(w, r, "count actor resets", err)
		return
	}
	if actorCount >= maxResetsPerActorWindow {
		writeError(w, http.StatusTooManyRequests, "too many password reset issuances; slow down")
		return
	}

	// Per-target cap: protects the target user's inbox from being flooded
	// regardless of who is issuing the resets.
	userCount, err := s.passwordReset.CountRecentForUser(r.Context(), user.DN, resetUserWindow)
	if err != nil {
		writeServerError(w, r, "count user resets", err)
		return
	}
	if userCount >= maxResetsPerUserWindow {
		writeError(w, http.StatusTooManyRequests, "a password reset was issued for this user recently; try again later")
		return
	}

	// Record who initiated the reset before any token is issued or email
	// sent, so an audit-DB hiccup cannot let a reset link leave the building
	// without a trail.
	if !s.auditMutating(w, r, audit.ActionUserUpdate, audit.ResourceUser, dn,
		map[string]interface{}{
			"action":                     "password_reset_sent",
			"email":                      user.Mail,
			audit.DetailsKeyResourceName: userDisplayName(user),
		}) {
		return
	}

	// Create password reset token
	token, err := s.passwordReset.CreateToken(r.Context(), user.DN, user.UID, user.Mail, session.UserDN)
	if err != nil {
		writeServerError(w, r, "create reset token", err)
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
		writeServerError(w, r, "send email", err)
		return
	}

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

	// Use the IP recorded by auditRequestInfoMiddleware, which itself reads
	// r.RemoteAddr after trustedProxyRealIPMiddleware has applied — so XFF
	// is honoured only for requests coming from a configured trusted proxy.
	clientIP := audit.RequestInfoFromContext(r.Context()).IPAddress

	// Atomically claim the token. ConsumeToken validates and marks it used in a
	// single statement, so two requests racing with the same token cannot both
	// proceed. We claim it *before* touching LDAP: a replay is then impossible,
	// at the cost of burning the token if the password change below fails.
	tokenInfo, err := s.passwordReset.ConsumeToken(r.Context(), token, clientIP)
	if err != nil {
		writeError(w, http.StatusNotFound, "invalid or expired token")
		return
	}

	// Get WHOIS info for the IP before audit so the row carries it.
	whoisInfo := mail.GetWhoisInfo(clientIP)

	// Record the intent before mutating LDAP — the action is unauthenticated,
	// so a lost trail would leave us without any record of who reset whose
	// password from where.
	if !s.auditMutatingWithActor(w, r, tokenInfo.UserDN, tokenInfo.UserUID,
		audit.ActionUserUpdate, audit.ResourceUser, tokenInfo.UserDN,
		map[string]interface{}{
			"action":    "password_reset_completed",
			"ip":        clientIP,
			"whoisInfo": whoisInfo,
		}) {
		return
	}

	// Change password in LDAP. The token is already spent at this point; if this
	// fails the user must request a fresh reset (the fail-closed direction).
	if err := s.ldapClient.ChangePassword(tokenInfo.UserDN, req.Password); err != nil {
		writeServerError(w, r, "change password", err)
		return
	}

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

	writeJSON(w, http.StatusOK, map[string]string{"message": "password changed successfully"})
}

// getAdminEmails returns the mail address of every member of the configured
// admin group. Called from the unauthenticated handleConfirmPasswordReset, so
// the implementation does targeted lookups (one search per admin) rather than
// a ListUsers() full scan per admin — the previous implementation was
// O(admins × all_users) and cheap to weaponise into a directory-wide DoS.
func (s *Server) getAdminEmails() []string {
	group, err := s.ldapClient.GetGroupByCN(s.config.App.AdminGroup)
	if err != nil {
		return nil
	}

	emails := make([]string, 0, len(group.MemberUIDs))
	for _, uid := range group.MemberUIDs {
		user, err := s.ldapClient.GetUserByUID(uid)
		if err != nil {
			continue
		}
		if user.Mail != "" {
			emails = append(emails, user.Mail)
		}
	}
	return emails
}
