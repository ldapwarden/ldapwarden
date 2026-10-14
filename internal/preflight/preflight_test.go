package preflight

import (
	"errors"
	"strings"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/config"
)

// validConfig passes every static check, so each case only changes what it is
// about.
func validConfig() *config.Config {
	return &config.Config{
		LDAP: config.LDAPConfig{
			URL: "ldaps://directory.example.org", TLSMode: "ssl",
			DefaultPolicyDN: "cn=default,ou=policies,dc=example,dc=org",
		},
		Session: config.SessionConfig{Secret: "a-session-secret-that-is-long-enough!"},
		App: config.AppConfig{
			PublicURL: "https://ldapwarden.example.org",
			Modules:   []string{"users", "groups", "policies"},
		},
		Mail:           config.MailConfig{SSL: "starttls"},
		ScheduledTasks: config.ScheduledTasksConfig{LockoutScan: "*/5 * * * *", TokensCleanup: "17 * * * *"},
	}
}

func findings(r *Report, level Level) []string {
	var out []string
	for _, f := range r.Findings {
		if f.Level == level {
			out = append(out, f.Message)
		}
	}
	return out
}

func TestStaticValidConfig(t *testing.T) {
	cfg := validConfig()
	cfg.LDAP.BindPass = "not-the-default"
	r := Static(cfg, nil)
	if r.Failed() {
		t.Fatalf("errors on a valid configuration: %v", findings(r, Error))
	}
	if warns := findings(r, Warn); len(warns) != 0 {
		t.Errorf("warnings on a valid configuration: %v", warns)
	}
}

func TestStaticFindings(t *testing.T) {
	cases := []struct {
		name   string
		adjust func(*config.Config) error // returns the load error, if any
		level  Level
		want   string
	}{
		{
			// The scheduler stops at the first spec it cannot parse, and every
			// task after it silently never runs.
			name:   "unparsable cron spec",
			adjust: func(c *config.Config) error { c.ScheduledTasks.TokensCleanup = "61 * * *"; return nil },
			level:  Error,
			want:   "LDAPWARDEN_SCHEDULED_TASKS_TOKENS_CLEANUP",
		},
		{
			name: "every joined load error is its own finding",
			adjust: func(c *config.Config) error {
				return errors.Join(errors.New("LDAP_BIND_PASS_FILE: missing"), errors.New("MAIL_PASSWORD_FILE: empty"))
			},
			level: Error,
			want:  "MAIL_PASSWORD_FILE",
		},
		{
			name:   "tls contradiction",
			adjust: func(c *config.Config) error { c.LDAP.URL = "ldap://directory.example.org"; return nil },
			level:  Error,
			want:   "needs an ldaps://",
		},
		{
			name: "plaintext directory",
			adjust: func(c *config.Config) error {
				c.LDAP.URL, c.LDAP.TLSMode = "ldap://directory.example.org", "none"
				return nil
			},
			level: Warn,
			want:  "not encrypted",
		},
		{
			name:   "verification off",
			adjust: func(c *config.Config) error { c.LDAP.TLSSkipVerify = true; return nil },
			level:  Warn,
			want:   "LDAP_TLS_SKIP_VERIFY",
		},
		{
			name:   "typo in a module",
			adjust: func(c *config.Config) error { c.App.Modules = append(c.App.Modules, "sudoers"); return nil },
			level:  Warn,
			want:   `unknown module "sudoers"`,
		},
		{
			name:   "unknown mail tls mode",
			adjust: func(c *config.Config) error { c.Mail.SSL = "tls"; return nil },
			level:  Error,
			want:   "MAIL_SSL",
		},
		{
			name:   "no default policy",
			adjust: func(c *config.Config) error { c.LDAP.DefaultPolicyDN = ""; return nil },
			level:  Warn,
			want:   "LDAP_DEFAULT_POLICY_DN",
		},
		{
			name:   "dev mode",
			adjust: func(c *config.Config) error { c.App.DevMode = true; return nil },
			level:  Warn,
			want:   "LDAPWARDEN_DEV_MODE",
		},
		{
			name:   "demo secret",
			adjust: func(c *config.Config) error { c.LDAP.BindPass = "admin"; return nil },
			level:  Error,
			want:   "LDAP_BIND_PASS",
		},
		{
			name:   "lockout scan off",
			adjust: func(c *config.Config) error { c.ScheduledTasks.LockoutScan = ""; return nil },
			level:  Warn,
			want:   "lockout scan is disabled",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.LDAP.BindPass = "not-the-default"
			loadErr := tc.adjust(cfg)
			r := Static(cfg, loadErr)
			got := findings(r, tc.level)
			for _, msg := range got {
				if strings.Contains(msg, tc.want) {
					if tc.level == Error && !r.Failed() {
						t.Error("an error finding did not fail the report")
					}
					return
				}
			}
			t.Errorf("no %s finding mentions %q; got %v", tc.level, tc.want, r.Findings)
		})
	}
}

func TestRedactRemovesDatabasePassword(t *testing.T) {
	got := redact(errors.New("cannot connect with hunter2 to db"), "postgres://u:hunter2@db:5432/x")
	if strings.Contains(got, "hunter2") {
		t.Errorf("password survived redaction: %q", got)
	}
}
