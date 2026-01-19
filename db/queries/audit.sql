-- name: CreateAuditLog :one
INSERT INTO audit_logs (actor_dn, actor_uid, action, resource_type, resource_dn, details, ip_address, user_agent)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListAuditLogs :many
SELECT * FROM audit_logs
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListAuditLogsByActor :many
SELECT * FROM audit_logs
WHERE actor_dn = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListAuditLogsByResource :many
SELECT * FROM audit_logs
WHERE resource_type = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListAuditLogsByAction :many
SELECT * FROM audit_logs
WHERE action = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountAuditLogs :one
SELECT COUNT(*) FROM audit_logs;
