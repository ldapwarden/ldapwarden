// Package preflight checks a configuration before the server runs with it.
//
// Most of what it reports, the server would also discover — but later, and
// quieter: a scheduled task whose cron spec does not parse stops every task
// registered after it, and the server logs one line and carries on; a missing
// admin group means nobody is admin, which shows up as a login that works and an
// admin page that is not there. `ldapwarden check-config` says all of it at once,
// with an exit code a deployment pipeline can act on.
package preflight

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"

	"github.com/ldapwarden/ldapwarden/internal/api"
	"github.com/ldapwarden/ldapwarden/internal/config"
	"github.com/ldapwarden/ldapwarden/internal/ldap"
	"github.com/ldapwarden/ldapwarden/internal/scheduler"
)

// Level grades a finding. Only errors fail the check: a warning is something the
// server runs with, but probably should not.
type Level string

const (
	OK    Level = "ok"
	Warn  Level = "warn"
	Error Level = "error"
)

// Finding is one thing the check established, good or bad.
type Finding struct {
	Level Level `json:"level"`
	// Area groups findings for reading: config, tls, secrets, schedule, ldap,
	// database.
	Area    string `json:"area"`
	Message string `json:"message"`
}

type Report struct {
	Findings []Finding `json:"findings"`
}

func (r *Report) add(level Level, area, format string, args ...any) {
	r.Findings = append(r.Findings, Finding{Level: level, Area: area, Message: fmt.Sprintf(format, args...)})
}

// addErrors records each error joined into err as its own finding, so a
// configuration with three problems reads as three lines.
func (r *Report) addErrors(level Level, area string, err error) {
	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		for _, e := range joined.Unwrap() {
			r.addErrors(level, area, e)
		}
		return
	}
	r.add(level, area, "%s", err.Error())
}

// Failed reports whether any finding is an error.
func (r *Report) Failed() bool {
	return slices.ContainsFunc(r.Findings, func(f Finding) bool { return f.Level == Error })
}

// knownModules are the values LDAPWARDEN_MODULES understands. An unknown one is
// ignored by the UI without a word, which is how a typo turns a section off.
var knownModules = []string{"users", "groups", "sudo", "policies", "netgroups"}

// Static checks what can be checked without connecting to anything. loadErr is
// what config.Load returned alongside cfg.
func Static(cfg *config.Config, loadErr error) *Report {
	r := &Report{}

	if loadErr != nil {
		r.addErrors(Error, "secrets", loadErr)
	}

	checkLDAPURL(r, cfg.LDAP.URL)

	if err := config.ValidateTLS(cfg.LDAP); err != nil {
		r.addErrors(Error, "tls", err)
	} else if _, err := ldap.TLSConfig(cfg.LDAP); err != nil {
		r.addErrors(Error, "tls", err)
	} else if cfg.LDAP.UsesTLS() {
		switch {
		case cfg.LDAP.TLSSkipVerify:
			r.add(Warn, "tls", "certificate verification is off (LDAP_TLS_SKIP_VERIFY): anyone on the path can impersonate the directory and collect the bind password")
		case cfg.LDAP.TLSCAFile != "":
			r.add(OK, "tls", "connections are encrypted and checked against %s", cfg.LDAP.TLSCAFile)
		default:
			r.add(OK, "tls", "connections are encrypted and checked against the system CAs")
		}
	} else if !strings.HasPrefix(cfg.LDAP.URL, "ldapi://") {
		r.add(Warn, "tls", "connections to the directory are not encrypted: the bind password and every user's password cross the network in clear")
	}

	if cfg.App.DevMode {
		r.add(Warn, "secrets", "LDAPWARDEN_DEV_MODE is on: the production checks on secrets, the public URL and CORS are skipped. Only for the bundled demo stack")
	} else if err := config.ValidateSecrets(cfg); err != nil {
		r.addErrors(Error, "secrets", err)
	} else {
		r.add(OK, "secrets", "no in-repo default secret is in use")
	}

	if _, err := api.ParseTrustedProxies(cfg.App.TrustedProxies); err != nil {
		r.add(Error, "config", "LDAPWARDEN_TRUSTED_PROXIES: %v", err)
	}

	for _, module := range cfg.App.Modules {
		if !slices.Contains(knownModules, module) {
			r.add(Warn, "config", "LDAPWARDEN_MODULES: unknown module %q is ignored (known: %s)", module, strings.Join(knownModules, ", "))
		}
	}

	switch cfg.Mail.SSL {
	case "none", "ssl", "starttls":
	default:
		r.add(Error, "config", "MAIL_SSL %q is not one of none, ssl, starttls", cfg.Mail.SSL)
	}

	if cfg.LDAP.DefaultPolicyDN == "" && slices.Contains(cfg.App.Modules, "policies") {
		r.add(Warn, "config", "LDAP_DEFAULT_POLICY_DN is not set: a lockout on an account without its own pwdPolicySubentry cannot be resolved, and is reported as \"unknown\" and counted as locked")
	}

	checkSchedules(r, cfg.ScheduledTasks)
	return r
}

func checkLDAPURL(r *Report, raw string) {
	u, err := url.Parse(raw)
	if err != nil {
		r.add(Error, "config", "LDAP_URL %q: %v", raw, err)
		return
	}
	switch u.Scheme {
	case "ldap", "ldaps", "ldapi":
	default:
		r.add(Error, "config", "LDAP_URL %q: the scheme must be ldap://, ldaps:// or ldapi://", raw)
	}
}

// checkSchedules parses every cron spec. The scheduler registers its tasks in
// order and gives up at the first spec it cannot parse, and the server only logs
// it: one typo silently disables that task and every one after it.
func checkSchedules(r *Report, tasks config.ScheduledTasksConfig) {
	specs := []struct{ name, value string }{
		{"LDAPWARDEN_SCHEDULED_TASKS_USERS_EXPIRATION", tasks.UsersExpiration},
		{"LDAPWARDEN_SCHEDULED_TASKS_PASSWORDS_EXPIRATION", tasks.PasswordsExpiration},
		{"LDAPWARDEN_SCHEDULED_TASKS_TOKENS_CLEANUP", tasks.TokensCleanup},
		{"LDAPWARDEN_SCHEDULED_TASKS_LOCKOUT_SCAN", tasks.LockoutScan},
	}
	valid := true
	for _, spec := range specs {
		if spec.value == "" {
			continue
		}
		if _, err := cron.ParseStandard(spec.value); err != nil {
			valid = false
			r.add(Error, "schedule", "%s %q: %v", spec.name, spec.value, err)
		}
	}
	if tasks.LockoutScan == "" {
		r.add(Warn, "schedule", "the lockout scan is disabled: a lockout the directory imposes leaves no record once pwdLockoutDuration lifts it")
	}
	if valid {
		r.add(OK, "schedule", "every scheduled task parses")
	}
}

// Connected checks the services the configuration points at: the database and
// the directory, as the identity the server will use. It appends to r.
func Connected(ctx context.Context, r *Report, cfg *config.Config) {
	checkDatabase(ctx, r, cfg.Database.URL)
	checkDirectory(r, cfg)
}

func checkDatabase(ctx context.Context, r *Report, databaseURL string) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// The URL carries the password: it is never repeated in a finding.
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		r.add(Error, "database", "DATABASE_URL does not parse: %v", redact(err, databaseURL))
		return
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		r.add(Error, "database", "PostgreSQL does not answer: %v", redact(err, databaseURL))
		return
	}
	r.add(OK, "database", "PostgreSQL answers")
}

// redact removes the URL's password from an error that may quote it.
func redact(err error, rawURL string) string {
	msg := err.Error()
	if u, parseErr := url.Parse(rawURL); parseErr == nil && u.User != nil {
		if password, ok := u.User.Password(); ok && password != "" {
			msg = strings.ReplaceAll(msg, password, "****")
		}
	}
	return msg
}

func checkDirectory(r *Report, cfg *config.Config) {
	client := ldap.NewClient(cfg.LDAP)
	defer client.Close()

	if err := client.Ping(); err != nil {
		r.add(Error, "ldap", "cannot read the base entry %s as %s: %v", cfg.LDAP.BaseDN, cfg.LDAP.BindDN, err)
		return
	}
	r.add(OK, "ldap", "bound to %s as %s and read %s", cfg.LDAP.URL, cfg.LDAP.BindDN, cfg.LDAP.BaseDN)

	for _, base := range client.MissingBaseDNs() {
		r.add(Warn, "ldap", "no %s base entry at %q (%s): requests for %s will be refused until it exists",
			base.Collection, base.DN, base.Setting, base.Collection)
	}

	// Admin is decided by membership of this group, by name: if it is not there,
	// nobody is admin — sign-in works and the admin page is simply missing.
	if _, err := client.GetGroupByCN(cfg.App.AdminGroup); err != nil {
		r.add(Error, "ldap", "LDAPWARDEN_ADMIN_GROUP %q: no such group under %s, so nobody would be admin (%v)",
			cfg.App.AdminGroup, client.GroupBaseDN(), err)
	} else {
		r.add(OK, "ldap", "admin group %q exists", cfg.App.AdminGroup)
	}

	if cfg.LDAP.DefaultPolicyDN != "" {
		if _, err := client.GetPasswordPolicy(cfg.LDAP.DefaultPolicyDN); err != nil {
			r.add(Error, "ldap", "LDAP_DEFAULT_POLICY_DN %q cannot be read: lockouts on accounts without their own policy would be resolved against nothing (%v)",
				cfg.LDAP.DefaultPolicyDN, err)
		} else {
			r.add(OK, "ldap", "default password policy %s is readable", cfg.LDAP.DefaultPolicyDN)
		}
	}

	if base := cfg.LDAP.AccessLogBase; base != "" {
		_, _, err := client.AccessLogChanges(time.Now().Add(-time.Minute), time.Time{}, 1)
		switch {
		case errors.Is(err, ldap.ErrAccessLogUnreadable):
			r.add(Warn, "ldap", "LDAP_ACCESSLOG_BASE %q: no such object. Either the accesslog overlay is not set up there, or %s may not read it; the directory changes view will say so too",
				base, cfg.LDAP.BindDN)
		case err != nil:
			r.add(Warn, "ldap", "LDAP_ACCESSLOG_BASE %q cannot be searched: %v", base, err)
		default:
			r.add(OK, "ldap", "the access log at %s is readable", base)
		}
	}

	if msg, warn := scheduler.LockoutCadenceWarning(cfg.ScheduledTasks.LockoutScan, client); warn {
		r.add(Warn, "schedule", "lockout scan: %s", msg)
	}
}
