-- Sessions move here from Redis, which held nothing else.
--
-- 001 created a sessions table for this, but the app never used it: sessions
-- went to Redis and the table stayed empty, its columns (ip_address,
-- user_agent) describing a design nobody wrote. It is replaced rather than
-- altered, since it cannot hold any rows worth keeping.
--
-- The session is stored whole, as the JSON the Redis store kept, so the
-- session shape can change without a migration. token_hash is the SHA-256 of
-- the cookie token — never the token itself. user_dn is kept apart to revoke
-- every session of an account at once (a lock, a demotion, a deletion).
DROP TABLE IF EXISTS sessions;

CREATE TABLE sessions (
    token_hash VARCHAR(64) PRIMARY KEY,
    user_dn VARCHAR(500) NOT NULL,
    data JSONB NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sessions_user_dn ON sessions(user_dn);
-- For the purge of expired rows, which Redis did by itself with a TTL.
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
