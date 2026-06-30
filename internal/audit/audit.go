package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ldapwarden/ldapwarden/internal/auth"
)

// Notifier is implemented by anything that can deliver a per-change audit
// notification (e.g. *mail.Mailer). Args are primitives (the change list is a
// slice of plain string maps) so implementers do not need to import this
// package.
type Notifier interface {
	SendAuditNotification(
		recipients []string,
		timestamp time.Time,
		actorUID, actorDN string,
		action, resourceType, resourceDN, resourceName string,
		changes []map[string]string,
		details map[string]interface{},
		ipAddress, userAgent string,
	) error
}

// FieldChange records a single attribute mutation for an update action. It is
// stored under details["changes"] by the handlers (so the diff is persisted in
// the audit_logs row too) and forwarded to the Notifier. Masked changes carry
// no Old/New values — used for secrets such as passwords, where the fact that
// the field changed is auditable but the value must never appear in an email.
type FieldChange struct {
	Field  string `json:"field"`
	Old    string `json:"old,omitempty"`
	New    string `json:"new,omitempty"`
	Masked bool   `json:"masked,omitempty"`
}

// DetailsKeyChanges and DetailsKeyResourceName are the reserved keys handlers
// use to pass the structured diff and a human-readable resource name through
// the details map. maybeNotify strips them back out before notifying.
const (
	DetailsKeyChanges      = "changes"
	DetailsKeyResourceName = "resourceName"
)

// RequestInfo carries the request-side metadata (IP, User-Agent) attached to
// audit log entries. It is set by the HTTP middleware and read in LogWithActor.
type RequestInfo struct {
	IPAddress string
	UserAgent string
}

type requestInfoCtxKey struct{}

// ContextWithRequestInfo returns ctx with the given request info attached.
func ContextWithRequestInfo(ctx context.Context, info RequestInfo) context.Context {
	return context.WithValue(ctx, requestInfoCtxKey{}, info)
}

// RequestInfoFromContext returns the request info previously stored, or a zero
// value if none is set (e.g. for scheduler-driven calls).
func RequestInfoFromContext(ctx context.Context) RequestInfo {
	if v, ok := ctx.Value(requestInfoCtxKey{}).(RequestInfo); ok {
		return v
	}
	return RequestInfo{}
}

type Action string

const (
	ActionLogin            Action = "login"
	ActionLoginFailed      Action = "login.failed"
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
	pool              *pgxpool.Pool
	notifier          Notifier
	notifyRecipients  []string
}

func NewLogger(pool *pgxpool.Pool, notifier Notifier, notifyRecipients []string) *Logger {
	return &Logger{
		pool:             pool,
		notifier:         notifier,
		notifyRecipients: notifyRecipients,
	}
}

// notifiableActions are the audit actions that should trigger an email when
// LDAPWARDEN_AUDIT_NOTIFY_EMAILS is configured. Login/logout, schema refresh
// and scheduler-driven notifications are intentionally excluded — only UI
// modifications to LDAP state are forwarded.
var notifiableActions = map[Action]struct{}{
	ActionUserCreate:       {},
	ActionUserUpdate:       {},
	ActionUserDelete:       {},
	ActionUserLock:         {},
	ActionUserUnlock:       {},
	ActionGroupCreate:      {},
	ActionGroupUpdate:      {},
	ActionGroupDelete:      {},
	ActionMemberAdd:        {},
	ActionMemberRemove:     {},
	ActionSudoRoleCreate:   {},
	ActionSudoRoleUpdate:   {},
	ActionSudoRoleDelete:   {},
	ActionSudoRoleUserAdd:  {},
	ActionSudoRoleUserDel:  {},
	ActionSudoRoleGroupAdd: {},
	ActionSudoRoleGroupDel: {},
	ActionPwdPolicyCreate:  {},
	ActionPwdPolicyUpdate:  {},
	ActionPwdPolicyDelete:  {},
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
	info := RequestInfoFromContext(ctx)

	var ipAddress, userAgent *string
	if info.IPAddress != "" {
		ipAddress = &info.IPAddress
	}
	if info.UserAgent != "" {
		userAgent = &info.UserAgent
	}

	_, err := l.pool.Exec(ctx, `
		INSERT INTO audit_logs (actor_dn, actor_uid, action, resource_type, resource_dn, details, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, actorDN, actorUID, string(action), string(resourceType), resourceDN, detailsJSON, ipAddress, userAgent)

	if err != nil {
		// Surface the failure on stderr with enough context that an operator
		// can reconstruct the lost row even when callers swallow the error
		// (the audit DB being unavailable is exactly when these traces are
		// most valuable). The error is also returned so strict callers can
		// refuse to mutate state without a recorded audit.
		log.Printf(
			"audit log INSERT failed: action=%s resource=%s resourceDn=%q actorUid=%q actorDn=%q ip=%q details=%s err=%v",
			action, resourceType, resourceDN, actorUID, actorDN, info.IPAddress, string(detailsJSON), err,
		)
		return fmt.Errorf("insert audit log: %w", err)
	}

	l.maybeNotify(action, actorDN, actorUID, resourceType, resourceDN, details, info)

	return nil
}

// maybeNotify dispatches an audit-notification email for modification actions
// when recipients are configured. The send runs in a goroutine so SMTP latency
// never blocks the calling HTTP handler; failures are logged.
func (l *Logger) maybeNotify(action Action, actorDN, actorUID string, resourceType ResourceType, resourceDN string, details map[string]interface{}, info RequestInfo) {
	if l.notifier == nil || len(l.notifyRecipients) == 0 {
		return
	}
	if _, ok := notifiableActions[action]; !ok {
		return
	}

	recipients := append([]string(nil), l.notifyRecipients...)
	timestamp := time.Now()

	resourceName, _ := details[DetailsKeyResourceName].(string)
	changes := changesToMaps(details[DetailsKeyChanges])

	go func() {
		// Never let a panic in the notification path (the mailer renders
		// attacker-influenced LDAP attributes through go-premailer/goquery)
		// escape this goroutine — a bare goroutine panic would crash the whole
		// process and drop every live session. A failed notification must stay
		// a logged non-event.
		defer func() {
			if r := recover(); r != nil {
				log.Printf("audit notification: panic recovered: %v", r)
			}
		}()

		if err := l.notifier.SendAuditNotification(
			recipients,
			timestamp,
			actorUID, actorDN,
			string(action), string(resourceType), resourceDN, resourceName,
			changes,
			details,
			info.IPAddress, info.UserAgent,
		); err != nil {
			log.Printf("audit notification: %v", err)
		}
	}()
}

// changesToMaps converts the typed FieldChange slice stored in details into the
// primitive []map[string]string the Notifier consumes, so the mail package
// stays free of an audit import. Masked changes are forwarded with only the
// field name and a "masked" flag; their values are deliberately dropped.
func changesToMaps(v interface{}) []map[string]string {
	changes, ok := v.([]FieldChange)
	if !ok || len(changes) == 0 {
		return nil
	}
	out := make([]map[string]string, 0, len(changes))
	for _, c := range changes {
		m := map[string]string{"field": c.Field}
		if c.Masked {
			m["masked"] = "true"
		} else {
			m["old"] = c.Old
			m["new"] = c.New
		}
		out = append(out, m)
	}
	return out
}

func (l *Logger) List(ctx context.Context, params ListParams) ([]LogEntry, int64, error) {
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 100 {
		params.Limit = 100
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	countQuery := `SELECT COUNT(*) FROM audit_logs WHERE 1=1`
	// ip_address is an INET column; pgx cannot scan INET into *string, so a row
	// with a non-null IP would fail the scan and 500 the whole listing (it only
	// ever worked when every visible row had a null IP). host() returns the
	// address as plain text, which scans cleanly.
	query := `SELECT id, actor_dn, actor_uid, action, resource_type, resource_dn, details, host(ip_address) AS ip_address, user_agent, created_at
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
