package rbac

import (
	"context"
	"slices"

	"github.com/ldapwarden/ldapwarden/internal/auth"
)

const (
	PermUsersRead    = "users:read"
	PermUsersWrite   = "users:write"
	PermUsersDelete  = "users:delete"
	PermGroupsRead   = "groups:read"
	PermGroupsWrite  = "groups:write"
	PermGroupsDelete = "groups:delete"
	PermAuditRead    = "audit:read"
	PermSchemaRead   = "schema:read"
	PermSchemaWrite  = "schema:write"
	PermSettingsRead = "settings:read"
	PermSettingsWrite = "settings:write"
)

type Role struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

var DefaultRoles = map[string]Role{
	"admin": {
		Name:        "admin",
		Description: "Full administrative access",
		Permissions: []string{
			PermUsersRead, PermUsersWrite, PermUsersDelete,
			PermGroupsRead, PermGroupsWrite, PermGroupsDelete,
			PermAuditRead, PermSchemaRead, PermSchemaWrite,
			PermSettingsRead, PermSettingsWrite,
		},
	},
	"readonly": {
		Name:        "readonly",
		Description: "Read-only access to directory",
		Permissions: []string{
			PermUsersRead, PermGroupsRead, PermSchemaRead,
		},
	},
}

type RBAC struct {
	roles      map[string]Role
	adminGroup string
}

func NewRBAC(adminGroup string) *RBAC {
	return &RBAC{
		roles:      DefaultRoles,
		adminGroup: adminGroup,
	}
}

func (r *RBAC) HasPermission(ctx context.Context, permission string) bool {
	session := auth.GetSessionFromContext(ctx)
	if session == nil {
		return false
	}

	return slices.Contains(session.Permissions, permission)
}

func (r *RBAC) HasAnyPermission(ctx context.Context, permissions ...string) bool {
	session := auth.GetSessionFromContext(ctx)
	if session == nil {
		return false
	}

	for _, perm := range permissions {
		if slices.Contains(session.Permissions, perm) {
			return true
		}
	}

	return false
}

func (r *RBAC) HasAllPermissions(ctx context.Context, permissions ...string) bool {
	session := auth.GetSessionFromContext(ctx)
	if session == nil {
		return false
	}

	for _, perm := range permissions {
		if !slices.Contains(session.Permissions, perm) {
			return false
		}
	}

	return true
}

func (r *RBAC) GetRole(name string) (Role, bool) {
	role, ok := r.roles[name]
	return role, ok
}

func (r *RBAC) IsAdmin(ctx context.Context) bool {
	session := auth.GetSessionFromContext(ctx)
	if session == nil {
		return false
	}

	return session.RoleName == "admin"
}

func GetSessionFromContext(ctx context.Context) *auth.Session {
	return auth.GetSessionFromContext(ctx)
}
