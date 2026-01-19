package passwordreset

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const TokenExpiry = 24 * time.Hour

type Token struct {
	ID          uuid.UUID
	UserDN      string
	UserUID     string
	UserEmail   string
	TokenHash   string
	ExpiresAt   time.Time
	Used        bool
	UsedAt      *time.Time
	UsedIP      *string
	CreatedAt   time.Time
	CreatedByDN string
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) CreateToken(ctx context.Context, userDN, userUID, userEmail, createdByDN string) (string, error) {
	// Generate a random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	// Hash the token for storage
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	expiresAt := time.Now().Add(TokenExpiry)

	_, err := s.pool.Exec(ctx, `
		INSERT INTO password_reset_tokens (user_dn, user_uid, user_email, token_hash, expires_at, created_by_dn)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, userDN, userUID, userEmail, tokenHash, expiresAt, createdByDN)

	if err != nil {
		return "", fmt.Errorf("create token: %w", err)
	}

	return token, nil
}

func (s *Service) ValidateToken(ctx context.Context, token string) (*Token, error) {
	// Hash the provided token
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	var t Token
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_dn, user_uid, user_email, token_hash, expires_at, used, used_at, used_ip, created_at, created_by_dn
		FROM password_reset_tokens
		WHERE token_hash = $1 AND expires_at > NOW() AND used = FALSE
	`, tokenHash).Scan(
		&t.ID, &t.UserDN, &t.UserUID, &t.UserEmail, &t.TokenHash,
		&t.ExpiresAt, &t.Used, &t.UsedAt, &t.UsedIP, &t.CreatedAt, &t.CreatedByDN,
	)

	if err != nil {
		return nil, fmt.Errorf("token not found or expired")
	}

	return &t, nil
}

func (s *Service) MarkTokenUsed(ctx context.Context, tokenID uuid.UUID, usedIP string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE password_reset_tokens
		SET used = TRUE, used_at = NOW(), used_ip = $2
		WHERE id = $1
	`, tokenID, usedIP)

	if err != nil {
		return fmt.Errorf("mark token used: %w", err)
	}

	return nil
}

func (s *Service) DeleteExpiredTokens(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM password_reset_tokens WHERE expires_at < NOW()`)
	if err != nil {
		return fmt.Errorf("delete expired tokens: %w", err)
	}
	return nil
}
