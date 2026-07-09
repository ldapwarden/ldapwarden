package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

type contextKey string

const (
	SessionContextKey contextKey = "session"
)

type Session struct {
	ID          string    `json:"id"`
	UserDN      string    `json:"userDn"`
	UserUID     string    `json:"userUid"`
	DisplayName string    `json:"displayName"`
	Mail        string    `json:"mail,omitempty"`
	RoleName    string    `json:"roleName"`
	Permissions []string  `json:"permissions"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

type AuthService struct {
	ldapClient   *ldap.Client
	sessionStore SessionStore
	sessionTTL   time.Duration
	adminGroup   string
}

type SessionStore interface {
	Create(ctx context.Context, session *Session, tokenHash string) error
	Get(ctx context.Context, tokenHash string) (*Session, error)
	Delete(ctx context.Context, tokenHash string) error
	DeleteByUserDN(ctx context.Context, userDN string) error
}

func NewAuthService(ldapClient *ldap.Client, store SessionStore, sessionTTL time.Duration, adminGroup string) *AuthService {
	return &AuthService{
		ldapClient:   ldapClient,
		sessionStore: store,
		sessionTTL:   sessionTTL,
		adminGroup:   adminGroup,
	}
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token   string   `json:"token"`
	Session *Session `json:"session"`
}

func (s *AuthService) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	// The login form accepts either a uid or an email address.
	user, err := s.ldapClient.GetUserByLogin(req.Username)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	if err := s.ldapClient.Bind(user.DN, req.Password); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	tokenHash := hashToken(token)

	// Check if user is in admin group
	roleName := "readonly"
	permissions := []string{"users:read", "groups:read", "schema:read"}

	groups, err := s.ldapClient.GetUserGroups(user.UID)
	if err == nil {
		for _, group := range groups {
			// LDAP CN matching is caseIgnoreMatch — "Admins" and "admins"
			// designate the same group, and the configured admin-group name
			// should not silently lose its meaning to a casing mismatch.
			if strings.EqualFold(group.CN, s.adminGroup) {
				roleName = "admin"
				permissions = []string{
					"users:read", "users:write", "users:delete",
					"groups:read", "groups:write", "groups:delete",
					"audit:read", "schema:read", "schema:write",
					"settings:read", "settings:write",
				}
				break
			}
		}
	}

	session := &Session{
		UserDN:      user.DN,
		UserUID:     user.UID,
		DisplayName: user.DisplayName,
		Mail:        user.Mail,
		RoleName:    roleName,
		Permissions: permissions,
		ExpiresAt:   time.Now().Add(s.sessionTTL),
	}

	if err := s.sessionStore.Create(ctx, session, tokenHash); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return &LoginResponse{
		Token:   token,
		Session: session,
	}, nil
}

// InvalidateUserSessions drops every session associated with userDN. Called by
// handlers that change a user's authorization-relevant state (account
// delete/lock, password reset, admin-group membership change) so that an
// already-cached session does not retain rights up to SESSION_TTL.
func (s *AuthService) InvalidateUserSessions(ctx context.Context, userDN string) error {
	return s.sessionStore.DeleteByUserDN(ctx, userDN)
}

func (s *AuthService) Logout(ctx context.Context, token string) error {
	tokenHash := hashToken(token)
	return s.sessionStore.Delete(ctx, tokenHash)
}

func (s *AuthService) ValidateToken(ctx context.Context, token string) (*Session, error) {
	tokenHash := hashToken(token)
	session, err := s.sessionStore.Get(ctx, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("invalid token")
	}

	if time.Now().After(session.ExpiresAt) {
		_ = s.sessionStore.Delete(ctx, tokenHash)
		return nil, fmt.Errorf("token expired")
	}

	return session, nil
}

func (s *AuthService) GetSession(ctx context.Context) *Session {
	return GetSessionFromContext(ctx)
}

func GetSessionFromContext(ctx context.Context) *Session {
	session, ok := ctx.Value(SessionContextKey).(*Session)
	if !ok {
		return nil
	}
	return session
}

func ContextWithSession(ctx context.Context, session *Session) context.Context {
	return context.WithValue(ctx, SessionContextKey, session)
}

func generateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
