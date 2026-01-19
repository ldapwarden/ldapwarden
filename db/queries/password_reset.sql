-- name: CreatePasswordResetToken :one
INSERT INTO password_reset_tokens (user_dn, user_uid, user_email, token_hash, expires_at, created_by_dn)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetPasswordResetToken :one
SELECT * FROM password_reset_tokens
WHERE token_hash = $1 AND expires_at > NOW() AND used = FALSE;

-- name: MarkPasswordResetTokenUsed :exec
UPDATE password_reset_tokens
SET used = TRUE, used_at = NOW(), used_ip = $2
WHERE id = $1;

-- name: DeleteExpiredPasswordResetTokens :exec
DELETE FROM password_reset_tokens WHERE expires_at < NOW();
