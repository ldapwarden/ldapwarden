package auth

import (
	"context"
	"encoding/json"
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

	userKey := s.prefix + "user:" + session.UserDN
	s.client.SAdd(ctx, userKey, tokenHash)
	s.client.Expire(ctx, userKey, ttl)

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

	for _, token := range tokens {
		key := s.prefix + token
		s.client.Del(ctx, key)
	}

	s.client.Del(ctx, userKey)

	return nil
}
