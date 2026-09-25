package scheduler

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store handles database operations for notification tracking
type Store struct {
	pool *pgxpool.Pool
}

// TaskRun represents a scheduled task execution record
type TaskRun struct {
	ID                uuid.UUID  `json:"id"`
	TaskName          string     `json:"taskName"`
	StartedAt         time.Time  `json:"startedAt"`
	CompletedAt       *time.Time `json:"completedAt,omitempty"`
	Status            string     `json:"status"`
	UsersChecked      int        `json:"usersChecked"`
	NotificationsSent int        `json:"notificationsSent"`
	Errors            []string   `json:"errors,omitempty"`
	TriggeredBy       string     `json:"triggeredBy"`
}

// TaskRunResult holds the result of a task execution
type TaskRunResult struct {
	RunID             uuid.UUID
	TaskName          string
	StartedAt         time.Time
	CompletedAt       time.Time
	UsersChecked      int
	NotificationsSent int
	Errors            []string
}

// NewStore creates a new notification store
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// CreateTaskRun creates a new task run record
func (s *Store) CreateTaskRun(ctx context.Context, taskName, triggeredBy string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO scheduled_task_runs (task_name, triggered_by)
		VALUES ($1, $2)
		RETURNING id
	`, taskName, triggeredBy).Scan(&id)
	return id, err
}

// CompleteTaskRun marks a task run as completed
func (s *Store) CompleteTaskRun(ctx context.Context, id uuid.UUID, usersChecked, notificationsSent int, errors []string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE scheduled_task_runs
		SET completed_at = NOW(), status = 'completed',
			users_checked = $2, notifications_sent = $3, errors = $4
		WHERE id = $1
	`, id, usersChecked, notificationsSent, errors)
	return err
}

// FailTaskRun marks a task run as failed
func (s *Store) FailTaskRun(ctx context.Context, id uuid.UUID, errorMsg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE scheduled_task_runs
		SET completed_at = NOW(), status = 'failed', errors = ARRAY[$2]
		WHERE id = $1
	`, id, errorMsg)
	return err
}

// LastTaskRun returns the most recent run of a task, or nil if it never ran.
// With scheduledOnly it skips manual runs, which say nothing about whether the
// schedule fires.
func (s *Store) LastTaskRun(ctx context.Context, taskName string, scheduledOnly bool) (*TaskRun, error) {
	runs, err := s.queryTaskRuns(ctx, `
		SELECT id, task_name, started_at, completed_at, status, users_checked,
			notifications_sent, errors, triggered_by
		FROM scheduled_task_runs
		WHERE task_name = $1 AND ($2 = false OR triggered_by = 'scheduler')
		ORDER BY started_at DESC
		LIMIT 1
	`, taskName, scheduledOnly)
	if err != nil || len(runs) == 0 {
		return nil, err
	}
	return &runs[0], nil
}

// WasNotificationSent checks if a notification was already sent
func (s *Store) WasNotificationSent(ctx context.Context, notifType, userDN, intervalKey string, expDate time.Time) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM expiration_notifications
		WHERE notification_type = $1 AND user_dn = $2 AND interval_key = $3
		AND DATE(expiration_date) = DATE($4)
	`, notifType, userDN, intervalKey, expDate).Scan(&count)
	return count > 0, err
}

// RecordNotification records a sent notification
func (s *Store) RecordNotification(ctx context.Context, notifType, userDN, userUID, intervalKey string, expDate time.Time, email string, runID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO expiration_notifications (notification_type, user_dn, user_uid, interval_key, expiration_date, recipient_email, task_run_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (notification_type, user_dn, interval_key, expiration_date) DO NOTHING
	`, notifType, userDN, userUID, intervalKey, expDate, email, runID)
	return err
}

// RecordLockoutSeen registers a lockout and reports whether this call is the one
// that discovered it.
//
// The insert is the test: only the run that actually created the row gets true, so
// two overlapping runs cannot both audit the same lockout, and a race is settled
// by the database rather than by a read-then-write window.
func (s *Store) RecordLockoutSeen(ctx context.Context, userDN, userUID string, lockedAt time.Time, runID uuid.UUID) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO lockout_events (user_dn, user_uid, locked_at, task_run_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_dn, locked_at) DO NOTHING
	`, userDN, userUID, lockedAt, runID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// PendingLockoutNotifications returns the lockouts that have been recorded but not
// yet emailed — this run's discoveries plus anything an earlier run could not send,
// which is what makes a mail outage a delay rather than a loss.
//
// Keyed the way the caller matches them: "<dn>\x00<RFC3339 instant>".
func (s *Store) PendingLockoutNotifications(ctx context.Context) (map[string]bool, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT user_dn, locked_at FROM lockout_events
		WHERE notified_at IS NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pending := make(map[string]bool)
	for rows.Next() {
		var dn string
		var lockedAt time.Time
		if err := rows.Scan(&dn, &lockedAt); err != nil {
			return nil, err
		}
		pending[LockoutKey(dn, lockedAt)] = true
	}
	return pending, rows.Err()
}

// LockoutKey identifies one lockout event, for matching directory entries against
// stored rows.
func LockoutKey(dn string, lockedAt time.Time) string {
	return dn + "\x00" + lockedAt.UTC().Format(time.RFC3339)
}

// MarkLockoutNotified records that the digest went out, and to whom.
func (s *Store) MarkLockoutNotified(ctx context.Context, userDN string, lockedAt time.Time, recipients []string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE lockout_events
		SET notified_at = NOW(), recipients = $3
		WHERE user_dn = $1 AND locked_at = $2
	`, userDN, lockedAt, recipients)
	return err
}

// PruneLockoutEvents drops the rows for lockouts the directory no longer holds,
// keeping exactly the ones listed. Empty lists mean nothing is locked anywhere, so
// nothing has to be remembered.
func (s *Store) PruneLockoutEvents(ctx context.Context, dns []string, lockedAt []time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM lockout_events
		WHERE (user_dn, locked_at) NOT IN (
			SELECT * FROM unnest($1::text[], $2::timestamptz[])
		)
	`, dns, lockedAt)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// GetTaskRuns returns recent task execution history
func (s *Store) GetTaskRuns(ctx context.Context, taskName string, limit int) ([]TaskRun, error) {
	return s.queryTaskRuns(ctx, `
		SELECT id, task_name, started_at, completed_at, status, users_checked, notifications_sent, errors, triggered_by
		FROM scheduled_task_runs
		WHERE ($1 = '' OR task_name = $1)
		ORDER BY started_at DESC
		LIMIT $2
	`, taskName, limit)
}

// queryTaskRuns runs a query selecting the scheduled_task_runs columns in
// TaskRun order.
func (s *Store) queryTaskRuns(ctx context.Context, query string, args ...any) ([]TaskRun, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []TaskRun
	for rows.Next() {
		var r TaskRun
		err := rows.Scan(&r.ID, &r.TaskName, &r.StartedAt, &r.CompletedAt, &r.Status, &r.UsersChecked, &r.NotificationsSent, &r.Errors, &r.TriggeredBy)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}
