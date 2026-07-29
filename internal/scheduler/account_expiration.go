package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ldapwarden/ldapwarden/internal/audit"
)

// runAccountExpirationTask checks for accounts about to expire and sends notifications
func (s *Scheduler) runAccountExpirationTask(ctx context.Context, triggeredBy string) (*TaskRunResult, error) {
	// Prevent concurrent runs
	s.mu.Lock()
	if s.running["users_expiration"] {
		s.mu.Unlock()
		return nil, fmt.Errorf("task already running")
	}
	s.running["users_expiration"] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running["users_expiration"] = false
		s.mu.Unlock()
	}()

	log.Printf("Starting account expiration check (triggered by: %s)", triggeredBy)

	// Create task run record
	runID, err := s.store.CreateTaskRun(ctx, "users_expiration", triggeredBy)
	if err != nil {
		return nil, fmt.Errorf("create task run: %w", err)
	}

	result := &TaskRunResult{
		RunID:     runID,
		TaskName:  "users_expiration",
		StartedAt: time.Now(),
	}

	// Without pwdEndTime the directory has no account expiry to notify about, and
	// the app does not offer to set one — say so once rather than reporting a
	// successful run that examined nothing meaningful.
	if !s.ldapClient.Capabilities().ScheduledExpiry {
		log.Printf("account expiration check skipped: directory has no pwdEndTime (OpenLDAP >= 2.5 required)")
		result.Errors = append(result.Errors, "directory does not support scheduled account expiry (pwdEndTime); nothing to check")
		_ = s.store.CompleteTaskRun(ctx, runID, 0, 0, result.Errors)
		return result, nil
	}

	// Get all users
	users, err := s.ldapClient.ListUsers()
	if err != nil {
		_ = s.store.FailTaskRun(ctx, runID, err.Error())
		return nil, fmt.Errorf("list users: %w", err)
	}

	// Get admin group members for notification
	adminEmails, err := s.getAdminEmails()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("get admin emails: %v", err))
	}

	if len(adminEmails) == 0 {
		result.Errors = append(result.Errors, "no admin emails found")
		_ = s.store.CompleteTaskRun(ctx, runID, 0, 0, result.Errors)
		return result, nil
	}

	now := time.Now()

	for _, user := range users {
		result.UsersChecked++

		// The account's expiry is pwdEndTime — the only value the directory
		// enforces. pwdAccountLockedTime is a lock (nothing to warn about: the
		// account is already disabled), and shadowExpire is read by PAM on the
		// hosts, not by slapd, so it cannot say whether this account still works.
		if user.PwdEndTime == "" {
			continue
		}
		expTime, err := parseLDAPTime(user.PwdEndTime)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("parse pwdEndTime for %s: %v", user.UID, err))
			continue
		}

		// Skip if expiration is in the past (already expired more than 1 day ago)
		if expTime.Before(now.Add(-24 * time.Hour)) {
			continue
		}

		// Find the most appropriate interval based on time remaining
		// We iterate from smallest to largest interval and pick the first match
		timeUntilExpiration := expTime.Sub(now)
		var selectedInterval *struct {
			Key      string
			Duration time.Duration
			Message  string
		}

		switch {
		case timeUntilExpiration <= 0:
			selectedInterval = &expirationIntervals[3] // "expired"
		case timeUntilExpiration <= 24*time.Hour:
			selectedInterval = &expirationIntervals[2] // "1_day"
		case timeUntilExpiration <= 7*24*time.Hour:
			selectedInterval = &expirationIntervals[1] // "1_week"
		case timeUntilExpiration <= 21*24*time.Hour:
			selectedInterval = &expirationIntervals[0] // "3_weeks"
		}

		if selectedInterval == nil {
			// More than 3 weeks away, no notification needed
			continue
		}

		// Check if already notified for this interval
		alreadySent, err := s.store.WasNotificationSent(ctx, "account_expiration", user.DN, selectedInterval.Key, expTime)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("check notification for %s: %v", user.UID, err))
			continue
		}
		if alreadySent {
			continue
		}

		// Send notification to all admins
		displayName := user.DisplayName
		if displayName == "" {
			displayName = user.CN
		}

		for _, email := range adminEmails {
			err = s.mailer.SendAccountExpirationNotification(email, user.UID, displayName, expTime, selectedInterval.Message)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("send email for %s to %s: %v", user.UID, email, err))
				continue
			}

			// Record notification (once per admin to track who was notified)
			_ = s.store.RecordNotification(ctx, "account_expiration", user.DN, user.UID, selectedInterval.Key, expTime, email, runID)
			result.NotificationsSent++
		}

		// Audit log the notification
		_ = s.auditLog.LogWithActor(ctx, "system", triggeredBy, "", audit.ActionAccountExpirationNotification, audit.ResourceUser, user.DN, map[string]interface{}{
			"userUid":        user.UID,
			"displayName":    displayName,
			"expirationDate": expTime.Format(time.RFC3339),
			"interval":       selectedInterval.Message,
			"recipientCount": len(adminEmails),
		})

		log.Printf("Sent account expiration notification for %s (%s) to %d admins", user.UID, selectedInterval.Message, len(adminEmails))
	}

	result.CompletedAt = time.Now()
	_ = s.store.CompleteTaskRun(ctx, runID, result.UsersChecked, result.NotificationsSent, result.Errors)

	log.Printf("Account expiration check completed: %d users checked, %d notifications sent", result.UsersChecked, result.NotificationsSent)

	return result, nil
}
