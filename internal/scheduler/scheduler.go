package scheduler

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"

	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/config"
	"github.com/ldapwarden/ldapwarden/internal/ldap"
	"github.com/ldapwarden/ldapwarden/internal/mail"
	"github.com/ldapwarden/ldapwarden/internal/passwordreset"
)

// Notification intervals for expiration warnings
var expirationIntervals = []struct {
	Key      string
	Duration time.Duration
	Message  string
}{
	{"3_weeks", 21 * 24 * time.Hour, "in 3 weeks"},
	{"1_week", 7 * 24 * time.Hour, "in 1 week"},
	{"1_day", 24 * time.Hour, "tomorrow"},
	{"expired", 0, "has expired"},
}

// Scheduler manages scheduled background tasks
type Scheduler struct {
	cron          *cron.Cron
	config        *config.Config
	ldapClient    *ldap.Client
	mailer        *mail.Mailer
	pool          *pgxpool.Pool
	store         *Store
	auditLog      *audit.Logger
	passwordReset *passwordreset.Service

	sessions SessionPurger

	mu      sync.Mutex
	running map[string]bool // Track running tasks to prevent overlap

	// When Start ran: the earliest a scheduled run can have happened in this
	// process, which TaskHealth needs for a task that has never run.
	startedAt time.Time
}

// SessionPurger removes expired sessions, for the hourly cleanup.
type SessionPurger interface {
	DeleteExpired(ctx context.Context) (int64, error)
}

// New creates a new scheduler instance
func New(cfg *config.Config, ldapClient *ldap.Client, mailer *mail.Mailer, pool *pgxpool.Pool, auditLog *audit.Logger, passwordReset *passwordreset.Service, sessions SessionPurger) *Scheduler {
	return &Scheduler{
		cron:          cron.New(),
		config:        cfg,
		ldapClient:    ldapClient,
		mailer:        mailer,
		pool:          pool,
		store:         NewStore(pool),
		auditLog:      auditLog,
		passwordReset: passwordReset,
		sessions:      sessions,
		running:       make(map[string]bool),
	}
}

// Start initializes and starts the scheduler
func (s *Scheduler) Start(ctx context.Context) error {
	// Register account expiration task
	if s.config.ScheduledTasks.UsersExpiration != "" {
		_, err := s.cron.AddFunc(s.config.ScheduledTasks.UsersExpiration, func() {
			log.Println("Cron triggered: users_expiration")
			if _, err := s.runAccountExpirationTask(context.Background(), "scheduler"); err != nil {
				log.Printf("Account expiration task error: %v", err)
			}
		})
		if err != nil {
			return fmt.Errorf("add users expiration task: %w", err)
		}
		log.Printf("Scheduled users expiration task: %s", s.config.ScheduledTasks.UsersExpiration)
	} else {
		log.Println("Users expiration task disabled (empty schedule)")
	}

	// Register password expiration task
	if s.config.ScheduledTasks.PasswordsExpiration != "" {
		_, err := s.cron.AddFunc(s.config.ScheduledTasks.PasswordsExpiration, func() {
			log.Println("Cron triggered: passwords_expiration")
			if _, err := s.runPasswordExpirationTask(context.Background(), "scheduler"); err != nil {
				log.Printf("Password expiration task error: %v", err)
			}
		})
		if err != nil {
			return fmt.Errorf("add passwords expiration task: %w", err)
		}
		log.Printf("Scheduled passwords expiration task: %s", s.config.ScheduledTasks.PasswordsExpiration)
	} else {
		log.Println("Passwords expiration task disabled (empty schedule)")
	}

	// Register the cleanup of expired tokens: password-reset tokens and
	// sessions. Both are already unusable past expires_at, but the row stays
	// until purged, growing the table indefinitely and obscuring forensic
	// queries with cold records. Sessions used to expire on their own in
	// Redis, by TTL; in the database this is what removes them. The job is
	// purely DML so we keep it lightweight (no task-run accounting) and run
	// it more often than the daily tasks.
	if (s.passwordReset != nil || s.sessions != nil) && s.config.ScheduledTasks.TokensCleanup != "" {
		_, err := s.cron.AddFunc(s.config.ScheduledTasks.TokensCleanup, func() {
			ctx := context.Background()
			if s.passwordReset != nil {
				if err := s.passwordReset.DeleteExpiredTokens(ctx); err != nil {
					log.Printf("Password reset cleanup error: %v", err)
				}
			}
			if s.sessions != nil {
				if _, err := s.sessions.DeleteExpired(ctx); err != nil {
					log.Printf("Session cleanup error: %v", err)
				}
			}
		})
		if err != nil {
			return fmt.Errorf("add tokens cleanup task: %w", err)
		}
		log.Printf("Scheduled expired tokens and sessions cleanup: %s", s.config.ScheduledTasks.TokensCleanup)
	}

	// Register the lockout scan. Unlike the expiration tasks this one is looking
	// for something the directory forgets: see runLockoutScanTask.
	if s.config.ScheduledTasks.LockoutScan != "" {
		_, err := s.cron.AddFunc(s.config.ScheduledTasks.LockoutScan, func() {
			log.Println("Cron triggered: lockouts_scan")
			if _, err := s.runLockoutScanTask(context.Background(), "scheduler"); err != nil {
				log.Printf("Lockout scan task error: %v", err)
			}
		})
		if err != nil {
			return fmt.Errorf("add lockout scan task: %w", err)
		}
		log.Printf("Scheduled lockout scan task: %s", s.config.ScheduledTasks.LockoutScan)
	} else {
		log.Println("Lockout scan task disabled (empty schedule)")
	}

	s.cron.Start()
	s.startedAt = time.Now()
	// Log next scheduled run times
	for _, entry := range s.cron.Entries() {
		log.Printf("Next cron run: %v", entry.Next)
	}
	return nil
}

// Stop gracefully stops the scheduler
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	log.Println("Scheduler stopped")
}

// TriggerAccountExpiration manually triggers the account expiration check
func (s *Scheduler) TriggerAccountExpiration(ctx context.Context, triggeredBy string) (*TaskRunResult, error) {
	return s.runAccountExpirationTask(ctx, "manual:"+triggeredBy)
}

// TriggerPasswordExpiration manually triggers the password expiration check
func (s *Scheduler) TriggerPasswordExpiration(ctx context.Context, triggeredBy string) (*TaskRunResult, error) {
	return s.runPasswordExpirationTask(ctx, "manual:"+triggeredBy)
}

// TriggerLockoutScan manually triggers the lockout scan
func (s *Scheduler) TriggerLockoutScan(ctx context.Context, triggeredBy string) (*TaskRunResult, error) {
	return s.runLockoutScanTask(ctx, "manual:"+triggeredBy)
}

// GetTaskRuns returns recent task execution history
func (s *Scheduler) GetTaskRuns(ctx context.Context, taskName string, limit int) ([]TaskRun, error) {
	return s.store.GetTaskRuns(ctx, taskName, limit)
}

// getAdminEmails retrieves email addresses of admin group members
func (s *Scheduler) getAdminEmails() ([]string, error) {
	adminGroup, err := s.ldapClient.GetGroupByCN(s.config.App.AdminGroup)
	if err != nil {
		return nil, fmt.Errorf("get admin group: %w", err)
	}

	var emails []string
	for _, uid := range adminGroup.MemberUIDs {
		user, err := s.ldapClient.GetUserByUID(uid)
		if err != nil {
			continue
		}
		if user.Mail != "" {
			emails = append(emails, user.Mail)
		}
	}

	return emails, nil
}

// parseLDAPTime parses LDAP generalized time format (20060102150405Z)
func parseLDAPTime(ldapTime string) (time.Time, error) {
	return time.Parse("20060102150405Z", ldapTime)
}
