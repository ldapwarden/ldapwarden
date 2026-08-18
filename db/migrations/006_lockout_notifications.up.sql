-- One row per lockout the directory posted itself, so each one is audited once
-- and emailed once.
--
-- The key is (user_dn, locked_at): a lockout is an event, not a state, and its
-- pwdAccountLockedTime value is what identifies it. A later lockout on the same
-- entry carries a different instant and is a different event.
--
-- Seen and notified are separate columns because they can fail independently: the
-- audit row is written from the directory alone and must not be repeated, while
-- the email depends on a mail server that may be down and has to be retried.
-- Merging them would either duplicate audit rows for the length of an SMTP
-- outage, or drop the alert entirely.
--
-- Rows are pruned by absence, not by age: every scan drops the ones whose
-- (user_dn, locked_at) no longer exists in the directory, so the table stays the
-- size of the currently locked set. Pruning by age would re-notify a lock that
-- outlived the retention window — and a lock with pwdLockoutDuration 0 outlives
-- any window.
CREATE TABLE lockout_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_dn VARCHAR(500) NOT NULL,
    user_uid VARCHAR(255) NOT NULL,
    locked_at TIMESTAMPTZ NOT NULL,
    seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notified_at TIMESTAMPTZ,
    recipients TEXT[],
    task_run_id UUID,

    CONSTRAINT uq_lockout_event UNIQUE (user_dn, locked_at)
);

CREATE INDEX idx_lockout_events_pending ON lockout_events(notified_at) WHERE notified_at IS NULL;
CREATE INDEX idx_lockout_events_user ON lockout_events(user_dn);
