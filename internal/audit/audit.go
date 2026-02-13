package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ldapwarden/ldapwarden/internal/auth"
)

type Action string

const (
	ActionLogin            Action = "login"
	ActionLogout           Action = "logout"
	ActionUserCreate       Action = "user.create"
	ActionUserUpdate       Action = "user.update"
	ActionUserDelete       Action = "user.delete"
	ActionUserLock         Action = "user.lock"
	ActionUserUnlock       Action = "user.unlock"
	ActionGroupCreate      Action = "group.create"
	ActionGroupUpdate      Action = "group.update"
	ActionGroupDelete      Action = "group.delete"
	ActionMemberAdd        Action = "group.member.add"
	ActionMemberRemove     Action = "group.member.remove"
	ActionSchemaRefresh    Action = "schema.refresh"
	ActionSudoRoleCreate   Action = "sudorole.create"
	ActionSudoRoleUpdate   Action = "sudorole.update"
	ActionSudoRoleDelete   Action = "sudorole.delete"
	ActionSudoRoleUserAdd   Action = "sudorole.user.add"
	ActionSudoRoleUserDel   Action = "sudorole.user.remove"
	ActionSudoRoleGroupAdd  Action = "sudorole.group.add"
	ActionSudoRoleGroupDel  Action = "sudorole.group.remove"
	ActionPwdPolicyCreate  Action = "pwdpolicy.create"
	ActionPwdPolicyUpdate  Action = "pwdpolicy.update"
	ActionPwdPolicyDelete  Action = "pwdpolicy.delete"
	ActionAccountExpirationNotification  Action = "notification.account_expiration"
	ActionPasswordExpirationNotification Action = "notification.password_expiration"
)

type ResourceType string

const (
	ResourceUser      ResourceType = "user"
	ResourceGroup     ResourceType = "group"
	ResourceSchema    ResourceType = "schema"
	ResourceSudoRole  ResourceType = "sudorole"
	ResourcePwdPolicy ResourceType = "pwdpolicy"
)

type LogEntry struct {
	ID           string                 `json:"id"`
	ActorDN      string                 `json:"actorDn"`
	ActorUID     string                 `json:"actorUid"`
	Action       Action                 `json:"action"`
	ResourceType ResourceType           `json:"resourceType"`
	ResourceDN   string                 `json:"resourceDn,omitempty"`
	Details      map[string]interface{} `json:"details,omitempty"`
	IPAddress    string                 `json:"ipAddress,omitempty"`
	UserAgent    string                 `json:"userAgent,omitempty"`
	CreatedAt    time.Time              `json:"createdAt"`
}

type ListParams struct {
	Limit        int
	Offset       int
	ActorDN      string
	ResourceType ResourceType
	Action       Action
}

type Logger struct {
	pool *pgxpool.Pool
}

func NewLogger(pool *pgxpool.Pool) *Logger {
	return &Logger{pool: pool}
}

func (l *Logger) Log(ctx context.Context, action Action, resourceType ResourceType, resourceDN string, details map[string]interface{}) error {
	session := auth.GetSessionFromContext(ctx)
	if session == nil {
		return fmt.Errorf("no session in context")
	}

	return l.LogWithActor(ctx, session.UserDN, session.UserUID, action, resourceType, resourceDN, details)
}

func (l *Logger) LogWithActor(ctx context.Context, actorDN, actorUID string, action Action, resourceType ResourceType, resourceDN string, details map[string]interface{}) error {
	detailsJSON, _ := json.Marshal(details)

	_, err := l.pool.Exec(ctx, `
		INSERT INTO audit_logs (actor_dn, actor_uid, action, resource_type, resource_dn, details)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, actorDN, actorUID, string(action), string(resourceType), resourceDN, detailsJSON)

	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}

	return nil
}

func (l *Logger) List(ctx context.Context, params ListParams) ([]LogEntry, int64, error) {
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 100 {
		params.Limit = 100
	}

	countQuery := `SELECT COUNT(*) FROM audit_logs WHERE 1=1`
	query := `SELECT id, actor_dn, actor_uid, action, resource_type, resource_dn, details, ip_address, user_agent, created_at
	          FROM audit_logs WHERE 1=1`

	args := []interface{}{}
	argNum := 1

	if params.ActorDN != "" {
		countQuery += fmt.Sprintf(" AND actor_dn = $%d", argNum)
		query += fmt.Sprintf(" AND actor_dn = $%d", argNum)
		args = append(args, params.ActorDN)
		argNum++
	}

	if params.ResourceType != "" {
		countQuery += fmt.Sprintf(" AND resource_type = $%d", argNum)
		query += fmt.Sprintf(" AND resource_type = $%d", argNum)
		args = append(args, string(params.ResourceType))
		argNum++
	}

	if params.Action != "" {
		countQuery += fmt.Sprintf(" AND action = $%d", argNum)
		query += fmt.Sprintf(" AND action = $%d", argNum)
		args = append(args, string(params.Action))
		argNum++
	}

	var total int64
	err := l.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count audit logs: %w", err)
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argNum, argNum+1)
	args = append(args, params.Limit, params.Offset)

	rows, err := l.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query audit logs: %w", err)
	}
	defer rows.Close()

	entries := make([]LogEntry, 0)
	for rows.Next() {
		var entry LogEntry
		var detailsJSON []byte
		var ipAddress, userAgent *string

		err := rows.Scan(
			&entry.ID, &entry.ActorDN, &entry.ActorUID,
			&entry.Action, &entry.ResourceType, &entry.ResourceDN,
			&detailsJSON, &ipAddress, &userAgent, &entry.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan audit log: %w", err)
		}

		if detailsJSON != nil {
			_ = json.Unmarshal(detailsJSON, &entry.Details)
		}
		if ipAddress != nil {
			entry.IPAddress = *ipAddress
		}
		if userAgent != nil {
			entry.UserAgent = *userAgent
		}

		entries = append(entries, entry)
	}

	return entries, total, nil
}
