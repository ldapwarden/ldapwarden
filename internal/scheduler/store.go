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

// GetTaskRuns returns recent task execution history
func (s *Store) GetTaskRuns(ctx context.Context, taskName string, limit int) ([]TaskRun, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, task_name, started_at, completed_at, status, users_checked, notifications_sent, errors, triggered_by
		FROM scheduled_task_runs
		WHERE ($1 = '' OR task_name = $1)
		ORDER BY started_at DESC
		LIMIT $2
	`, taskName, limit)
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
	return runs, nil
}
