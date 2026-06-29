package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisSessionStore struct {
	client *redis.Client
	prefix string
}

func NewRedisSessionStore(client *redis.Client) *RedisSessionStore {
	return &RedisSessionStore{
		client: client,
		prefix: "ldapwarden:session:",
	}
}

func (s *RedisSessionStore) Create(ctx context.Context, session *Session, tokenHash string) error {
	key := s.prefix + tokenHash

	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		return fmt.Errorf("session already expired")
	}

	if err := s.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("store session: %w", err)
	}

	// The per-user index is what DeleteByUserDN walks to revoke every live
	// session for a user (account lock/delete, admin demotion). If SAdd fails,
	// the session would be stored but invisible to revocation — roll it back
	// rather than leave behind a token nothing can kill.
	userKey := s.prefix + "user:" + session.UserDN
	if err := s.client.SAdd(ctx, userKey, tokenHash).Err(); err != nil {
		_ = s.client.Del(ctx, key).Err()
		return fmt.Errorf("index session for user: %w", err)
	}
	// A missing TTL on the index set is benign housekeeping (its members are
	// removed on logout/expiry), so a failure here must not fail the login.
	_ = s.client.Expire(ctx, userKey, ttl).Err()

	return nil
}

func (s *RedisSessionStore) Get(ctx context.Context, tokenHash string) (*Session, error) {
	key := s.prefix + tokenHash

	data, err := s.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("session not found")
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

func (s *RedisSessionStore) Delete(ctx context.Context, tokenHash string) error {
	key := s.prefix + tokenHash

	session, err := s.Get(ctx, tokenHash)
	if err == nil {
		userKey := s.prefix + "user:" + session.UserDN
		s.client.SRem(ctx, userKey, tokenHash)
	}

	if err := s.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

func (s *RedisSessionStore) DeleteByUserDN(ctx context.Context, userDN string) error {
	userKey := s.prefix + "user:" + userDN

	tokens, err := s.client.SMembers(ctx, userKey).Result()
	if err != nil {
		return fmt.Errorf("get user sessions: %w", err)
	}

	// Aggregate failures rather than stopping at the first, and surface them:
	// a partial revocation must not look like success to the caller, or a
	// locked/deleted/demoted user could keep a live session.
	var errs []error
	for _, token := range tokens {
		if err := s.client.Del(ctx, s.prefix+token).Err(); err != nil {
			errs = append(errs, fmt.Errorf("delete session %s: %w", token, err))
		}
	}
	if err := s.client.Del(ctx, userKey).Err(); err != nil {
		errs = append(errs, fmt.Errorf("delete user session index: %w", err))
	}

	return errors.Join(errs...)
}
