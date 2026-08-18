-- The roles / user_roles tables and the sessions.role_id column were
-- introduced for a DB-driven RBAC scheme that the application never wired
-- up: authorization is derived at login time from LDAP group membership
-- (see internal/auth/auth.go). Keeping these tables around misled
-- operators who tried to grep them for "who is admin" and gave the
-- audit trail a misleading shape. Drop them.

ALTER TABLE sessions DROP COLUMN IF EXISTS role_id;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS roles;
