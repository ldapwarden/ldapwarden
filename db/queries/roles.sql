-- name: GetRoleByName :one
SELECT * FROM roles WHERE name = $1;

-- name: GetRoleByID :one
SELECT * FROM roles WHERE id = $1;

-- name: ListRoles :many
SELECT * FROM roles ORDER BY name;

-- name: GetUserRole :one
SELECT r.* FROM roles r
JOIN user_roles ur ON r.id = ur.role_id
WHERE ur.user_dn = $1;

-- name: AssignUserRole :one
INSERT INTO user_roles (user_dn, role_id, assigned_by)
VALUES ($1, $2, $3)
ON CONFLICT (user_dn) DO UPDATE SET role_id = $2, assigned_by = $3
RETURNING *;

-- name: RemoveUserRole :exec
DELETE FROM user_roles WHERE user_dn = $1;
