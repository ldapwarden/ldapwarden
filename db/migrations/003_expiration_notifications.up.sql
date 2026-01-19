-- Track sent expiration notifications to avoid duplicates
CREATE TABLE expiration_notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_type VARCHAR(50) NOT NULL,  -- 'account_expiration' or 'password_expiration'
    user_dn VARCHAR(500) NOT NULL,
    user_uid VARCHAR(255) NOT NULL,
    interval_key VARCHAR(50) NOT NULL,  -- '3_weeks', '1_week', '1_day', 'expired'
    expiration_date TIMESTAMPTZ NOT NULL,
    recipient_email VARCHAR(255) NOT NULL,
    sent_at TIMESTAMPTZ DEFAULT NOW(),
    task_run_id UUID,  -- Links to a specific task execution

    -- Composite unique constraint to prevent duplicate notifications
    CONSTRAINT uq_notification_per_interval UNIQUE (notification_type, user_dn, interval_key, expiration_date)
);

CREATE INDEX idx_expiration_notifications_user ON expiration_notifications(user_dn);
CREATE INDEX idx_expiration_notifications_type ON expiration_notifications(notification_type);
CREATE INDEX idx_expiration_notifications_sent ON expiration_notifications(sent_at);

-- Task execution history for UI visibility
CREATE TABLE scheduled_task_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_name VARCHAR(100) NOT NULL,  -- 'users_expiration' or 'passwords_expiration'
    started_at TIMESTAMPTZ DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    status VARCHAR(50) NOT NULL DEFAULT 'running',  -- 'running', 'completed', 'failed'
    users_checked INT DEFAULT 0,
    notifications_sent INT DEFAULT 0,
    errors TEXT[],  -- Array of error messages
    triggered_by VARCHAR(100) DEFAULT 'scheduler'  -- 'scheduler' or 'manual:<user>'
);

CREATE INDEX idx_task_runs_name ON scheduled_task_runs(task_name);
CREATE INDEX idx_task_runs_started ON scheduled_task_runs(started_at DESC);
