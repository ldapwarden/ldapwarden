package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrSessionNotFound is returned by Get for a token with no live session.
var ErrSessionNotFound = errors.New("session not found")

// PostgresSessionStore keeps sessions in the application database.
//
// They used to live in Redis, which the app ran for nothing else. A session is
// read once per request, which for an admin tool used by a handful of people
// is no load worth a second datastore, and keeping them next to the audit log
// means one service to run, back up and secure instead of two.
type PostgresSessionStore struct {
	pool *pgxpool.Pool
}

func NewPostgresSessionStore(pool *pgxpool.Pool) *PostgresSessionStore {
	return &PostgresSessionStore{pool: pool}
}

// Ping reports whether the store is reachable, for the readiness probe. It is
// the same database as the audit log now, so the two checks can only fail
// together; the probe keeps them apart because they gate different things
// (signing in, and every mutation).
func (s *PostgresSessionStore) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *PostgresSessionStore) Create(ctx context.Context, session *Session, tokenHash string) error {
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO sessions (token_hash, user_dn, data, expires_at)
		VALUES ($1, $2, $3, $4)
	`, tokenHash, session.UserDN, data, session.ExpiresAt)
	if err != nil {
		return fmt.Errorf("store session: %w", err)
	}
	return nil
}

// Get returns the live session for a token. An expired row is treated as
// absent here, whether or not the purge has removed it yet: expiry is decided
// by expires_at, not by the purge schedule.
func (s *PostgresSessionStore) Get(ctx context.Context, tokenHash string) (*Session, error) {
	var data []byte
	err := s.pool.QueryRow(ctx, `
		SELECT data FROM sessions
		WHERE token_hash = $1 AND expires_at > NOW()
	`, tokenHash).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	return &session, nil
}

func (s *PostgresSessionStore) Delete(ctx context.Context, tokenHash string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteByUserDN revokes every session of an account. One statement, so a
// revocation is all or nothing: the Redis store deleted them one key at a time
// and had to aggregate partial failures, where a locked, deleted or demoted
// user could keep whichever session was missed.
func (s *PostgresSessionStore) DeleteByUserDN(ctx context.Context, userDN string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE user_dn = $1`, userDN); err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}
	return nil
}

// DeleteByUserDNExcept revokes every session of an account but the one whose
// token hashes to keepTokenHash: the one the user is acting from when they
// change their own password.
func (s *PostgresSessionStore) DeleteByUserDNExcept(ctx context.Context, userDN, keepTokenHash string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE user_dn = $1 AND token_hash <> $2`, userDN, keepTokenHash); err != nil {
		return fmt.Errorf("delete other user sessions: %w", err)
	}
	return nil
}

// DeleteExpired purges the rows past their expiry and reports how many went.
// Redis dropped them on its own with a TTL; here they would otherwise pile up,
// harmless — Get ignores them — but growing forever.
func (s *PostgresSessionStore) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= NOW()`)
	if err != nil {
		return 0, fmt.Errorf("purge expired sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}
