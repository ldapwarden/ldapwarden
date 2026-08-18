package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/ldap"
	"github.com/ldapwarden/ldapwarden/internal/mail"
)

// lockoutScanTask is the task name recorded in scheduled_task_runs.
const lockoutScanTask = "lockouts_scan"

// lockoutDigestLimit caps how many accounts one email describes in full. A
// spraying run can lock every account it has a username for, and a message
// listing two hundred of them is not read by anyone. The rest are counted, and
// all of them are in the audit log.
const lockoutDigestLimit = 25

// runLockoutScanTask notifies about the accounts the directory locked by itself.
//
// This is a poll, and it has to be, because ppolicy does not announce anything:
// it writes pwdAccountLockedTime during a failed bind and deletes it during the
// first bind after pwdLockoutDuration elapses. A lockout is therefore visible for
// at least that duration and no less — which is what makes a scan reliable, and
// what makes its interval a correctness property rather than a preference. The
// run reports the mismatch when the schedule is slower than the shortest policy
// duration, because the symptom otherwise is silence, and silence reads as "no
// incidents".
func (s *Scheduler) runLockoutScanTask(ctx context.Context, triggeredBy string) (*TaskRunResult, error) {
	s.mu.Lock()
	if s.running[lockoutScanTask] {
		s.mu.Unlock()
		return nil, fmt.Errorf("task already running")
	}
	s.running[lockoutScanTask] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running[lockoutScanTask] = false
		s.mu.Unlock()
	}()

	log.Printf("Starting lockout scan (triggered by: %s)", triggeredBy)

	runID, err := s.store.CreateTaskRun(ctx, lockoutScanTask, triggeredBy)
	if err != nil {
		return nil, fmt.Errorf("create task run: %w", err)
	}

	result := &TaskRunResult{
		RunID:     runID,
		TaskName:  lockoutScanTask,
		StartedAt: time.Now(),
	}

	locked, truncated, err := s.ldapClient.SelfLockedAccounts()
	if err != nil {
		_ = s.store.FailTaskRun(ctx, runID, err.Error())
		return nil, fmt.Errorf("list locked accounts: %w", err)
	}
	result.UsersChecked = len(locked)
	if truncated {
		// The set was cut short by the server, so an account may be locked without
		// appearing here. Never silently: the point of this task is that nothing
		// goes unreported.
		result.Errors = append(result.Errors,
			"the directory limited the search: some locked accounts may not have been examined")
	}
	if msg, ok := s.lockoutCadenceWarning(); ok {
		result.Errors = append(result.Errors, msg)
	}

	// Forget the lockouts the directory no longer holds, so the same entry locking
	// again reads as a new event. Pruning first also keeps the table at the size of
	// the currently locked set rather than of everything that ever happened.
	s.pruneLockoutState(ctx, locked, result)

	// Record and audit what has not been seen before. The audit row is written from
	// the directory alone: it is the copy that outlives ppolicy's own evidence, so
	// it must not wait on a mail server being reachable.
	discovered := 0
	for _, account := range locked {
		lockedAt, ok := lockoutInstant(account)
		if !ok {
			result.Errors = append(result.Errors,
				fmt.Sprintf("unreadable lock timestamp on %s: %q", account.UID, account.Lockout.LockedAt))
			continue
		}
		isNew, err := s.store.RecordLockoutSeen(ctx, account.DN, account.UID, lockedAt, runID)
		if err != nil {
			// A database that cannot answer must not be read as "already handled":
			// that would drop the alert. Fail the run instead.
			_ = s.store.FailTaskRun(ctx, runID, err.Error())
			return nil, fmt.Errorf("record lockout for %s: %w", account.UID, err)
		}
		if isNew {
			s.auditLockout(ctx, triggeredBy, account)
			discovered++
		}
	}

	// What still needs an email: this run's discoveries plus anything an earlier run
	// recorded but could not send, which is what makes a mail outage a delay rather
	// than a loss.
	pending, err := s.store.PendingLockoutNotifications(ctx)
	if err != nil {
		_ = s.store.FailTaskRun(ctx, runID, err.Error())
		return nil, fmt.Errorf("read pending lockout notifications: %w", err)
	}
	unnotified := make([]ldap.LockedAccount, 0, len(pending))
	for _, account := range locked {
		if lockedAt, ok := lockoutInstant(account); ok && pending[LockoutKey(account.DN, lockedAt)] {
			unnotified = append(unnotified, account)
		}
	}

	if len(unnotified) > 0 {
		recipients, err := s.lockoutRecipients()
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
		}
		if len(recipients) == 0 {
			result.Errors = append(result.Errors,
				fmt.Sprintf("%d lockout(s) recorded in the audit log but not emailed: no recipients (set LDAPWARDEN_AUDIT_NOTIFY_EMAILS or give the admin group members an email address)",
					len(unnotified)))
		} else {
			s.sendLockoutDigest(ctx, recipients, unnotified, result)
		}
	}

	result.CompletedAt = time.Now()
	_ = s.store.CompleteTaskRun(ctx, runID, result.UsersChecked, result.NotificationsSent, result.Errors)

	log.Printf("Lockout scan completed: %d locked account(s) seen, %d newly discovered, %d notifications sent",
		len(locked), discovered, result.NotificationsSent)

	return result, nil
}

// pruneLockoutState drops the stored lockouts the directory no longer holds.
func (s *Scheduler) pruneLockoutState(ctx context.Context, locked []ldap.LockedAccount, result *TaskRunResult) {
	dns := make([]string, 0, len(locked))
	instants := make([]time.Time, 0, len(locked))
	for _, account := range locked {
		if lockedAt, ok := lockoutInstant(account); ok {
			dns = append(dns, account.DN)
			instants = append(instants, lockedAt)
		}
	}
	if _, err := s.store.PruneLockoutEvents(ctx, dns, instants); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("prune lockout state: %v", err))
	}
}

// sendLockoutDigest emails the lockouts awaiting notification and records who was
// told.
func (s *Scheduler) sendLockoutDigest(ctx context.Context, recipients []string, fresh []ldap.LockedAccount, result *TaskRunResult) {
	shown, hidden := fresh, 0
	if len(shown) > lockoutDigestLimit {
		hidden = len(fresh) - lockoutDigestLimit
		shown = shown[:lockoutDigestLimit]
	}

	events := make([]mail.LockoutEvent, 0, len(shown))
	for _, account := range shown {
		events = append(events, lockoutEvent(account))
	}

	if err := s.mailer.SendLockoutDigest(recipients, events, hidden, time.Now()); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("send lockout digest: %v", err))
		return
	}

	// Marked only once the mail went out, so a broken mail server leaves the event
	// pending and the next run retries it instead of recording it as reported.
	for _, account := range fresh {
		lockedAt, ok := lockoutInstant(account)
		if !ok {
			continue
		}
		if err := s.store.MarkLockoutNotified(ctx, account.DN, lockedAt, recipients); err != nil {
			result.Errors = append(result.Errors,
				fmt.Sprintf("record lockout notification for %s: %v", account.UID, err))
			continue
		}
		// Counted per recipient, as the expiration tasks do.
		result.NotificationsSent += len(recipients)
	}
}

// auditLockout records one lockout, with the failed-bind timestamps in the
// details: ppolicy deletes them at the account's next sign-in, so this row is
// where they survive.
func (s *Scheduler) auditLockout(ctx context.Context, triggeredBy string, account ldap.LockedAccount) {
	attempts := make([]string, 0, len(account.FailureTimes))
	for _, t := range account.FailureTimes {
		attempts = append(attempts, t.UTC().Format(time.RFC3339))
	}

	details := map[string]interface{}{
		"userUid":       account.UID,
		"displayName":   account.DisplayName,
		"lockedAt":      account.Lockout.LockedAt,
		"lockState":     string(account.Lockout.State),
		"failedBinds":   attempts,
		"attemptsCount": len(attempts),
	}
	if account.Lockout.LiftsAt != "" {
		details["liftsAt"] = account.Lockout.LiftsAt
	}
	if account.Lockout.PolicyDN != "" {
		details["policyDn"] = account.Lockout.PolicyDN
	}
	if spread, ok := attemptSpread(account.FailureTimes); ok {
		details["attemptSpread"] = spread.String()
	}

	_ = s.auditLog.LogWithActor(ctx, "system", triggeredBy, "",
		audit.ActionUserLockout, audit.ResourceUser, account.DN, details)
}

// lockoutRecipients returns who hears about a lockout: the audit notification
// list when configured, since this is the same audience as every other
// security-relevant change, and the admin group otherwise so the feature does
// something useful without extra configuration.
func (s *Scheduler) lockoutRecipients() ([]string, error) {
	if len(s.config.App.AuditNotifyEmails) > 0 {
		return s.config.App.AuditNotifyEmails, nil
	}
	emails, err := s.getAdminEmails()
	if err != nil {
		return nil, fmt.Errorf("get admin emails: %w", err)
	}
	return emails, nil
}

// lockoutCadenceWarning reports a schedule that cannot see what it is looking
// for. A lockout is visible for pwdLockoutDuration; scan less often than that and
// lockouts pass unnoticed between two runs.
func (s *Scheduler) lockoutCadenceWarning() (string, bool) {
	spec := s.config.ScheduledTasks.LockoutScan
	if spec == "" {
		return "", false
	}
	shortest, ok := s.ldapClient.MinLockoutDuration()
	if !ok {
		// No policy lifts a lockout on its own, so every lockout stays visible and
		// any cadence catches it.
		return "", false
	}

	schedule, err := cron.ParseStandard(spec)
	if err != nil {
		return "", false
	}
	// Two consecutive firings measure the interval, whatever the expression says.
	first := schedule.Next(time.Now())
	interval := schedule.Next(first).Sub(first)
	if interval <= shortest {
		return "", false
	}

	return fmt.Sprintf(
		"this task runs every %s but the shortest pwdLockoutDuration is %s: a lockout can start and expire between two runs, and go unreported. Schedule it more often than %s (LDAPWARDEN_SCHEDULED_TASKS_LOCKOUT_SCAN)",
		interval, shortest, shortest), true
}

// lockoutInstant is the lock's identity as a time. Absent when the directory's
// value could not be read, which leaves the event unidentifiable — reported
// rather than papered over, since a wrong identity would either re-notify
// forever or hide a real lockout.
func lockoutInstant(account ldap.LockedAccount) (time.Time, bool) {
	if account.Lockout == nil || account.LockedAt.IsZero() {
		return time.Time{}, false
	}
	return account.LockedAt, true
}

// attemptSpread is how long the recorded failed binds took, from first to last.
// It is what separates a user mistyping a password from a script: the count is
// always pwdMaxFailure, the spread is not.
func attemptSpread(times []time.Time) (time.Duration, bool) {
	if len(times) < 2 {
		return 0, false
	}
	return times[len(times)-1].Sub(times[0]).Round(time.Second), true
}

// lockoutEvent renders one account for the digest.
func lockoutEvent(account ldap.LockedAccount) mail.LockoutEvent {
	event := mail.LockoutEvent{
		UID:         account.UID,
		DisplayName: account.DisplayName,
		Lifted:      account.Lockout.State == ldap.LockStateLifted,
	}

	if lockedAt, ok := lockoutInstant(account); ok {
		event.LockedAt = formatLockoutTime(lockedAt)
	} else {
		event.LockedAt = account.Lockout.LockedAt
	}
	if account.Lockout.LiftsAt != "" {
		if liftsAt, err := time.Parse(time.RFC3339, account.Lockout.LiftsAt); err == nil {
			event.LiftsAt = formatLockoutTime(liftsAt)
		}
	}

	for _, t := range account.FailureTimes {
		event.Attempts = append(event.Attempts, formatLockoutTime(t))
	}
	if spread, ok := attemptSpread(account.FailureTimes); ok {
		event.Spread = spread.String()
	}
	// Fewer recorded attempts than it takes to lock means ppolicy had already
	// trimmed the older ones out of its counting window, so the list understates
	// what happened and the email should not present it as complete.
	event.Partial = account.MaxFailure > 0 &&
		len(account.FailureTimes) > 0 &&
		len(account.FailureTimes) < account.MaxFailure

	return event
}

// formatLockoutTime renders an instant for a human reading an email, in UTC
// because that is what the directory stores and what two administrators in
// different time zones can compare.
func formatLockoutTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05 MST")
}
