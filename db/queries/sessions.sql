-- name: CreateSession :one
INSERT INTO sessions (user_dn, user_uid, role_id, token_hash, ip_address, user_agent, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetSessionByToken :one
SELECT s.*, r.name as role_name, r.permissions
FROM sessions s
JOIN roles r ON s.role_id = r.id
WHERE s.token_hash = $1 AND s.expires_at > NOW();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at < NOW();

-- name: DeleteUserSessions :exec
DELETE FROM sessions WHERE user_dn = $1;
