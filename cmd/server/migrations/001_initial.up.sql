-- LDAP connection configurations
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

-- Roles for RBAC
CREATE TABLE roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    permissions JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Insert default roles
INSERT INTO roles (name, description, permissions) VALUES
    ('admin', 'Full administrative access', '["users:read", "users:write", "users:delete", "groups:read", "groups:write", "groups:delete", "audit:read", "schema:read", "schema:write", "settings:read", "settings:write"]'),
    ('readonly', 'Read-only access to directory', '["users:read", "groups:read", "schema:read"]');

-- User sessions
CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_dn VARCHAR(500) NOT NULL,
    user_uid VARCHAR(255) NOT NULL,
    role_id UUID REFERENCES roles(id),
    token_hash VARCHAR(64) UNIQUE NOT NULL,
    ip_address INET,
    user_agent TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_sessions_token_hash ON sessions(token_hash);
CREATE INDEX idx_sessions_user_dn ON sessions(user_dn);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);

-- User role assignments (maps LDAP users to application roles)
CREATE TABLE user_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_dn VARCHAR(500) UNIQUE NOT NULL,
    role_id UUID REFERENCES roles(id) NOT NULL,
    assigned_by VARCHAR(500),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_user_roles_user_dn ON user_roles(user_dn);

-- Audit logs (append-only)
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_dn VARCHAR(500) NOT NULL,
    actor_uid VARCHAR(255) NOT NULL,
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    resource_dn VARCHAR(500),
    details JSONB,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_actor_dn ON audit_logs(actor_dn);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);
CREATE INDEX idx_audit_logs_resource_type ON audit_logs(resource_type);
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at DESC);

-- Cached LDAP schema
CREATE TABLE schema_cache (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ldap_connection_id UUID REFERENCES ldap_connections(id),
    object_classes JSONB NOT NULL DEFAULT '{}',
    attribute_types JSONB NOT NULL DEFAULT '{}',
    cached_at TIMESTAMPTZ DEFAULT NOW()
);

-- Prevent updates/deletes on audit_logs
CREATE OR REPLACE FUNCTION prevent_audit_modification()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Audit logs cannot be modified or deleted';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER audit_logs_immutable
    BEFORE UPDATE OR DELETE ON audit_logs
    FOR EACH ROW
    EXECUTE FUNCTION prevent_audit_modification();
