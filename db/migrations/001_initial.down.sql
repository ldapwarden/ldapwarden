DROP TRIGGER IF EXISTS audit_logs_immutable ON audit_logs;
DROP FUNCTION IF EXISTS prevent_audit_modification();
DROP TABLE IF EXISTS schema_cache;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS ldap_connections;
