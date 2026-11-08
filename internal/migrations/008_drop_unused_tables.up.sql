-- ldap_connections and schema_cache were created by 001 for a multi-backend
-- design the application never implemented: the LDAP server comes from the
-- environment (LDAP_URL, LDAP_BIND_DN, ...), and the schema is read from the
-- directory and cached in process. Nothing has ever read or written either
-- table, so they hold no rows worth keeping. Like the role tables 004 dropped,
-- they only suggested a design that does not exist.
--
-- schema_cache goes first: its ldap_connection_id references ldap_connections.

DROP TABLE IF EXISTS schema_cache;
DROP TABLE IF EXISTS ldap_connections;
