CREATE TABLE password_reset_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_dn VARCHAR(500) NOT NULL,
    user_uid VARCHAR(100) NOT NULL,
    user_email VARCHAR(255) NOT NULL,
    token_hash VARCHAR(64) UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used BOOLEAN DEFAULT FALSE,
    used_at TIMESTAMPTZ,
    used_ip VARCHAR(45),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    created_by_dn VARCHAR(500) NOT NULL
);

CREATE INDEX idx_prt_token_hash ON password_reset_tokens(token_hash);
CREATE INDEX idx_prt_expires_at ON password_reset_tokens(expires_at);
