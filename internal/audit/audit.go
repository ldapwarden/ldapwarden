package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
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
		actorUID, actorName, actorDN string,
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

type suppressNotifyCtxKey struct{}

// ContextSuppressingNotify marks ctx so that audit rows logged under it are
// still written but do NOT trigger a per-change notification email. Used by the
// bulk-import path, which records one audit row per created entry but sends a
// single summary email instead of one per row.
func ContextSuppressingNotify(ctx context.Context) context.Context {
	return context.WithValue(ctx, suppressNotifyCtxKey{}, true)
}

func notifySuppressed(ctx context.Context) bool {
	v, _ := ctx.Value(suppressNotifyCtxKey{}).(bool)
	return v
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
	ActionUserRename       Action = "user.rename"
	ActionGroupCreate      Action = "group.create"
	ActionGroupUpdate      Action = "group.update"
	ActionGroupDelete      Action = "group.delete"
	ActionMemberAdd        Action = "group.member.add"
	ActionMemberRemove     Action = "group.member.remove"
	ActionSchemaRefresh    Action = "schema.refresh"
	ActionSudoRoleCreate   Action = "sudorole.create"
	ActionSudoRoleUpdate   Action = "sudorole.update"
	ActionSudoRoleDelete   Action = "sudorole.delete"
	ActionSudoRoleUserAdd  Action = "sudorole.user.add"
	ActionSudoRoleUserDel  Action = "sudorole.user.remove"
	ActionSudoRoleGroupAdd Action = "sudorole.group.add"
	ActionSudoRoleGroupDel Action = "sudorole.group.remove"
	ActionNetgroupCreate   Action = "netgroup.create"
	ActionNetgroupUpdate   Action = "netgroup.update"
	ActionNetgroupDelete   Action = "netgroup.delete"
	ActionPwdPolicyCreate  Action = "pwdpolicy.create"
	ActionPwdPolicyUpdate  Action = "pwdpolicy.update"
	ActionPwdPolicyDelete  Action = "pwdpolicy.delete"
	// ActionUserLockout is the directory locking an account by itself, after
	// pwdMaxFailure failed binds — as opposed to ActionUserLock, which is an
	// administrator doing it through the app. Recorded by the lockout scan, with
	// the failed-bind timestamps in the details: ppolicy erases them at the next
	// bind, so the audit row is the only lasting record of them.
	ActionUserLockout                    Action = "user.lockout"
	ActionPolicySelfTest                 Action = "policy.selftest"
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
	ResourceNetgroup  ResourceType = "netgroup"
	ResourceDirectory ResourceType = "directory"
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
	// ActorUID matches the actor by uid rather than DN — what the UI's actor
	// filter offers, since that is what the table shows.
	ActorUID string
	// Search is a case-insensitive substring match over the actor uid and the
	// resource DN: "which entries mention jdoe", regardless of whether they were
	// the actor or the target.
	Search string
	// From/To bound created_at (inclusive). Zero values mean unbounded.
	From time.Time
	To   time.Time
}

// where builds the shared filter clause for List and Export so the two cannot
// drift — an export that does not match the view it was taken from is worse
// than no export.
func (p ListParams) where() (clause string, args []interface{}) {
	argNum := 1
	add := func(sql string, value interface{}) {
		clause += fmt.Sprintf(sql, argNum)
		args = append(args, value)
		argNum++
	}

	if p.ActorDN != "" {
		add(" AND actor_dn = $%d", p.ActorDN)
	}
	if p.ActorUID != "" {
		add(" AND actor_uid = $%d", p.ActorUID)
	}
	if p.ResourceType != "" {
		add(" AND resource_type = $%d", string(p.ResourceType))
	}
	if p.Action != "" {
		add(" AND action = $%d", string(p.Action))
	}
	if p.Search != "" {
		// ILIKE with a leading wildcard cannot use a btree index; acceptable
		// here because the operator is narrowing an already-filtered view, and
		// the result is always bounded by LIMIT.
		clause += fmt.Sprintf(" AND (actor_uid ILIKE $%d OR resource_dn ILIKE $%d)", argNum, argNum)
		args = append(args, "%"+p.Search+"%")
		argNum++
	}
	if !p.From.IsZero() {
		add(" AND created_at >= $%d", p.From)
	}
	if !p.To.IsZero() {
		add(" AND created_at <= $%d", p.To)
	}
	return clause, args
}

// selectColumns is shared by List and Export.
//
// host(ip_address) because ip_address is an INET column: pgx cannot scan INET
// into *string, so a row with a non-null IP would fail the scan and 500 the
// whole listing (it only ever worked when every visible row had a null IP).
const selectColumns = `SELECT id, actor_dn, actor_uid, action, resource_type, resource_dn, details, host(ip_address) AS ip_address, user_agent, created_at
	          FROM audit_logs WHERE 1=1`

type Logger struct {
	pool             *pgxpool.Pool
	notifier         Notifier
	notifyRecipients []string
}

// Ping reports whether the audit database is reachable, for the readiness probe.
// It matters more than it looks: auditMutating refuses every mutation when the log
// is unavailable, so an app that cannot reach Postgres is read-only.
func (l *Logger) Ping(ctx context.Context) error {
	return l.pool.Ping(ctx)
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
//
// ActionUserLockout is excluded for a different reason: it is emailed, but as one
// digest per scan rather than one message per account, because a password spray
// produces a dozen at once. Adding it here would send both.
var notifiableActions = map[Action]struct{}{
	ActionUserCreate:       {},
	ActionUserUpdate:       {},
	ActionUserDelete:       {},
	ActionUserLock:         {},
	ActionUserUnlock:       {},
	ActionUserRename:       {},
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

	return l.LogWithActor(ctx, session.UserDN, session.UserUID, session.DisplayName, action, resourceType, resourceDN, details)
}

// LogWithActor records an audit entry for an explicit actor. actorName is the
// actor's human-readable display name (may be empty); it is not persisted (the
// DN and uid are the stored identity) but is forwarded to the notifier so the
// email can read "Performed by: Lionel Porcheron (lionel)".
func (l *Logger) LogWithActor(ctx context.Context, actorDN, actorUID, actorName string, action Action, resourceType ResourceType, resourceDN string, details map[string]interface{}) error {
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

	if !notifySuppressed(ctx) {
		l.maybeNotify(action, actorDN, actorUID, actorName, resourceType, resourceDN, details, info)
	}

	return nil
}

// maybeNotify dispatches an audit-notification email for modification actions
// when recipients are configured. The send runs in a goroutine so SMTP latency
// never blocks the calling HTTP handler; failures are logged.
func (l *Logger) maybeNotify(action Action, actorDN, actorUID, actorName string, resourceType ResourceType, resourceDN string, details map[string]interface{}, info RequestInfo) {
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
			actorUID, actorName, actorDN,
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

	filter, args := params.where()

	var total int64
	err := l.pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs WHERE 1=1`+filter, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count audit logs: %w", err)
	}

	query := selectColumns + filter +
		fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, params.Limit, params.Offset)

	rows, err := l.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query audit logs: %w", err)
	}
	defer rows.Close()

	entries, err := scanEntries(rows)
	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

// exportMaxRows bounds an export so one request cannot stream an unbounded
// table into memory (on either side of the connection).
const exportMaxRows = 10000

// Export returns up to exportMaxRows entries matching the same filters as List,
// oldest first — the order an audit trail is read in.
func (l *Logger) Export(ctx context.Context, params ListParams) ([]LogEntry, error) {
	filter, args := params.where()

	query := selectColumns + filter +
		fmt.Sprintf(" ORDER BY created_at ASC LIMIT $%d", len(args)+1)
	args = append(args, exportMaxRows)

	rows, err := l.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query audit logs for export: %w", err)
	}
	defer rows.Close()

	return scanEntries(rows)
}

// Actors returns the distinct actors that appear in the log, most recent first,
// so the UI can offer them as a filter instead of asking for an exact uid.
func (l *Logger) Actors(ctx context.Context) ([]string, error) {
	rows, err := l.pool.Query(ctx, `
		SELECT actor_uid
		FROM audit_logs
		GROUP BY actor_uid
		ORDER BY MAX(created_at) DESC
		LIMIT 200`)
	if err != nil {
		return nil, fmt.Errorf("query audit actors: %w", err)
	}
	defer rows.Close()

	actors := make([]string, 0)
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, fmt.Errorf("scan audit actor: %w", err)
		}
		actors = append(actors, uid)
	}
	return actors, rows.Err()
}

func scanEntries(rows pgx.Rows) ([]LogEntry, error) {
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
			return nil, fmt.Errorf("scan audit log: %w", err)
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
	return entries, rows.Err()
}
