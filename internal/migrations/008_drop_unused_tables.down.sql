-- Re-create the unused tables exactly as 001 defined them, for a rollback to a
-- binary that still expects the 001 schema. The application does not query
-- them, so they come back empty.

CREATE TABLE ldap_connections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    url VARCHAR(500) NOT NULL,
    base_dn VARCHAR(500) NOT NULL,
    bind_dn VARCHAR(500),
    bind_password_encrypted BYTEA,
    user_ou VARCHAR(255) DEFAULT 'ou=People',
    group_ou VARCHAR(255) DEFAULT 'ou=Groups',
    use_tls BOOLEAN DEFAULT FALSE,
    is_default BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE schema_cache (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ldap_connection_id UUID REFERENCES ldap_connections(id),
    object_classes JSONB NOT NULL DEFAULT '{}',
    attribute_types JSONB NOT NULL DEFAULT '{}',
    cached_at TIMESTAMPTZ DEFAULT NOW()
);
