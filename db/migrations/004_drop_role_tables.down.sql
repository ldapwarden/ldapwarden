-- Re-create the dead RBAC tables for symmetry with the up migration.
-- The application does not query them; restoring them here only matters
-- if an operator needs to roll back to a binary that still expected the
-- 001 schema. Seed data matches the original 001 insert.

CREATE TABLE roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    permissions JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

INSERT INTO roles (name, description, permissions) VALUES
    ('admin', 'Full administrative access', '["users:read", "users:write", "users:delete", "groups:read", "groups:write", "groups:delete", "audit:read", "schema:read", "schema:write", "settings:read", "settings:write"]'),
    ('readonly', 'Read-only access to directory', '["users:read", "groups:read", "schema:read"]');

ALTER TABLE sessions ADD COLUMN role_id UUID REFERENCES roles(id);

CREATE TABLE user_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_dn VARCHAR(500) UNIQUE NOT NULL,
    role_id UUID REFERENCES roles(id) NOT NULL,
    assigned_by VARCHAR(500),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_user_roles_user_dn ON user_roles(user_dn);
