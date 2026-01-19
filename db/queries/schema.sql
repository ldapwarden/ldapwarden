-- name: GetSchemaCache :one
SELECT * FROM schema_cache
WHERE ldap_connection_id = $1
ORDER BY cached_at DESC
LIMIT 1;

-- name: UpsertSchemaCache :one
INSERT INTO schema_cache (ldap_connection_id, object_classes, attribute_types)
VALUES ($1, $2, $3)
ON CONFLICT (id) DO UPDATE SET
    object_classes = $2,
    attribute_types = $3,
    cached_at = NOW()
RETURNING *;

-- name: CreateSchemaCache :one
INSERT INTO schema_cache (ldap_connection_id, object_classes, attribute_types)
VALUES ($1, $2, $3)
RETURNING *;

-- name: DeleteSchemaCache :exec
DELETE FROM schema_cache WHERE ldap_connection_id = $1;
