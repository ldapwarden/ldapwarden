package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// runPasswordExpirationTask checks for passwords about to expire and sends notifications
func (s *Scheduler) runPasswordExpirationTask(ctx context.Context, triggeredBy string) (*TaskRunResult, error) {
	// Prevent concurrent runs
	s.mu.Lock()
	if s.running["passwords_expiration"] {
		s.mu.Unlock()
		return nil, fmt.Errorf("task already running")
	}
	s.running["passwords_expiration"] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running["passwords_expiration"] = false
		s.mu.Unlock()
	}()

	log.Printf("Starting password expiration check (triggered by: %s)", triggeredBy)

	runID, err := s.store.CreateTaskRun(ctx, "passwords_expiration", triggeredBy)
	if err != nil {
		return nil, fmt.Errorf("create task run: %w", err)
	}

	result := &TaskRunResult{
		RunID:     runID,
		TaskName:  "passwords_expiration",
		StartedAt: time.Now(),
	}

	users, err := s.ldapClient.ListUsers()
	if err != nil {
		_ = s.store.FailTaskRun(ctx, runID, err.Error())
		return nil, fmt.Errorf("list users: %w", err)
	}

	// Cache password policies
	policies, err := s.ldapClient.ListPasswordPolicies()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("list policies: %v", err))
	}
	policyMap := make(map[string]*ldap.PasswordPolicy)
	for i := range policies {
		policyMap[policies[i].DN] = &policies[i]
	}

	now := time.Now()

	for _, user := range users {
		result.UsersChecked++

		if user.Mail == "" {
			continue // Can't notify without email
		}

		// Determine password expiration time
		var expTime time.Time
		var hasExpiration bool

		// Method 1: ppolicy (pwdChangedTime + pwdMaxAge from policy)
		if user.PwdChangedTime != "" && user.PwdPolicySubentry != "" {
			if policy, ok := policyMap[user.PwdPolicySubentry]; ok && policy.PwdMaxAge > 0 {
				changedTime, err := parseLDAPTime(user.PwdChangedTime)
				if err == nil {
					expTime = changedTime.Add(time.Duration(policy.PwdMaxAge) * time.Second)
					hasExpiration = true
				}
			}
		}

		// Method 2: shadow (shadowLastChange + shadowMax)
		// Shadow dates are days since Unix epoch (Jan 1, 1970)
		if !hasExpiration && user.ShadowLastChange > 0 && user.ShadowMax > 0 && user.ShadowMax < 99999 {
			lastChange := time.Unix(int64(user.ShadowLastChange)*86400, 0)
			expTime = lastChange.Add(time.Duration(user.ShadowMax) * 24 * time.Hour)
			hasExpiration = true
		}

		if !hasExpiration {
			continue
		}

		// Skip if already expired more than 1 day ago
		if expTime.Before(now.Add(-24 * time.Hour)) {
			continue
		}

		// Find the most appropriate interval based on time remaining
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
		alreadySent, err := s.store.WasNotificationSent(ctx, "password_expiration", user.DN, selectedInterval.Key, expTime)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("check notification for %s: %v", user.UID, err))
			continue
		}
		if alreadySent {
			continue
		}

		displayName := user.DisplayName
		if displayName == "" {
			displayName = user.CN
		}

		// Send notification to the user
		err = s.mailer.SendPasswordExpirationNotification(user.Mail, user.UID, displayName, expTime, selectedInterval.Message)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("send email for %s: %v", user.UID, err))
			continue
		}

		_ = s.store.RecordNotification(ctx, "password_expiration", user.DN, user.UID, selectedInterval.Key, expTime, user.Mail, runID)
		result.NotificationsSent++

		// Audit log the notification
		_ = s.auditLog.LogWithActor(ctx, "system", triggeredBy, "", audit.ActionPasswordExpirationNotification, audit.ResourceUser, user.DN, map[string]interface{}{
			"userUid":        user.UID,
			"displayName":    displayName,
			"expirationDate": expTime.Format(time.RFC3339),
			"interval":       selectedInterval.Message,
			"recipient":      user.Mail,
		})

		log.Printf("Sent password expiration notification to %s (%s)", user.UID, selectedInterval.Message)
	}

	result.CompletedAt = time.Now()
	_ = s.store.CompleteTaskRun(ctx, runID, result.UsersChecked, result.NotificationsSent, result.Errors)

	log.Printf("Password expiration check completed: %d users checked, %d notifications sent", result.UsersChecked, result.NotificationsSent)

	return result, nil
}
