-- Back to the unused table 001 created, as 004 left it. Sessions do not go
-- back to Redis: the version this rolls back to keeps them there itself, so
-- everyone signs in again.
DROP TABLE IF EXISTS sessions;

CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_dn VARCHAR(500) NOT NULL,
    user_uid VARCHAR(255) NOT NULL,
    token_hash VARCHAR(64) UNIQUE NOT NULL,
    ip_address INET,
    user_agent TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_sessions_token_hash ON sessions(token_hash);
CREATE INDEX idx_sessions_user_dn ON sessions(user_dn);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
