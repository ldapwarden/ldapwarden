-- name: CreateSession :one
INSERT INTO sessions (user_dn, user_uid, token_hash, ip_address, user_agent, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetSessionByToken :one
SELECT * FROM sessions
WHERE token_hash = $1 AND expires_at > NOW();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at < NOW();

-- name: DeleteUserSessions :exec
DELETE FROM sessions WHERE user_dn = $1;
