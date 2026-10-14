package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Defaults that ValidateSecrets refuses outside dev mode. Centralised here so
// the values used to populate Config.Load and the values rejected by
// ValidateSecrets cannot drift apart.
const (
	defaultSessionSecret = "change-me-in-production-32bytes!"
	defaultLDAPBindPass  = "admin"
	minSessionSecretLen  = 32
)

type Config struct {
	Server         ServerConfig
	Database       DatabaseConfig
	LDAP           LDAPConfig
	Session        SessionConfig
	App            AppConfig
	Mail           MailConfig
	ScheduledTasks ScheduledTasksConfig
}

type ScheduledTasksConfig struct {
	UsersExpiration     string // Cron format, empty = disabled
	PasswordsExpiration string // Cron format, empty = disabled
	TokensCleanup       string // Cron format, empty = disabled. Purges expired password_reset_tokens and sessions rows.
	// LockoutScan looks for accounts the directory locked by itself. Its interval
	// is a correctness property, not a preference: ppolicy keeps a lockout visible
	// for pwdLockoutDuration and then erases it, so a slower scan misses lockouts
	// (the task reports the mismatch when it sees one). Cron format, empty =
	// disabled.
	LockoutScan string
}

type AppConfig struct {
	AdminGroup        string
	Organization      string
	PublicURL         string
	Modules           []string // High-level modules: users, groups, sudo, policies, netgroups
	UsersObjects      []string // LDAP objectClasses for users
	GroupsObjects     []string // LDAP objectClasses for groups
	AuditNotifyEmails []string // Recipients for per-change audit emails (empty disables the feature)
	TrustedProxies    []string // CIDR list of reverse proxies allowed to set X-Forwarded-For / X-Real-IP (empty = headers ignored)
	CORSOrigins       []string // Origins allowed by the CORS middleware. "*" is refused outside dev because the API uses credentialed requests.
	DevMode           bool     // When true, skips ValidateSecrets — only intended for the bundled docker-compose stack
}

type MailConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
	FromName string // optional display name for the From header
	SSL      string // "none", "starttls", "ssl"
}

type ServerConfig struct {
	Host string
	Port int
}

type DatabaseConfig struct {
	URL string
}

type LDAPConfig struct {
	URL       string
	BaseDN    string
	BindDN    string
	BindPass  string
	UserOU    string
	GroupOU   string
	SudoersOU string
	PpolicyOU string
	// NetgroupOU holds nisNetgroup entries. Separate from GroupOU because a
	// netgroup is not a posixGroup and the two are searched by object class:
	// mixing them in one OU works, but makes every listing pay for the other's
	// entries.
	NetgroupOU string
	// DefaultPolicyDN names the password policy the directory applies to entries
	// that carry no pwdPolicySubentry of their own — i.e. what olcPPolicyDefault
	// is set to on the ppolicy overlay.
	//
	// It has to be configured because it cannot be discovered: olcPPolicyDefault
	// lives in cn=config, which an ordinary bind identity cannot read (see
	// Client.probePolicyEnforcement). Without it the app cannot tell how long a
	// lockout lasts for most entries, and reports such a lock as "unknown"
	// instead of claiming it either holds or has lapsed.
	DefaultPolicyDN string
	MinUID          int
	MinGID          int
	TLSMode         string // "none", "ssl", "starttls"
	TLSSkipVerify   bool   // Skip certificate verification (for self-signed certs)
	// TLSCAFile is a PEM bundle of the certificate authorities trusted to sign
	// the directory's certificate. When set it *replaces* the system pool, as
	// TLS_CACERT does in ldap.conf: a private CA is the reason to set it, and
	// adding every public CA on top would trust certificates nobody meant to.
	TLSCAFile string
	// TLSCertFile and TLSKeyFile are the client certificate presented to a
	// directory that requires one (olcTLSVerifyClient demand). Both or neither.
	TLSCertFile     string
	TLSKeyFile      string
	SearchSizeLimit int // Max entries returned by UI list/search endpoints (0 = no limit)
	// Timeout bounds every single LDAP operation: the dial, the bind and each
	// request on an established connection.
	//
	// go-ldap applies no deadline of its own, so without this a request whose
	// response never arrives — a connection silently dropped by a firewall, a
	// server wedged on an unindexed search — waits for ever. That is not a slow
	// page but a permanently stuck one: the goroutine keeps whatever lock it
	// holds, and the handlers have no context to cancel it.
	//
	// Kept below the 30s the browser allows a request, so a directory that has
	// stopped answering produces an error the operator can read instead of a
	// client-side abort with nothing behind it. Raise it for a directory large
	// enough that a legitimate paged count runs longer.
	Timeout time.Duration
}

type SessionConfig struct {
	Secret string
	TTL    time.Duration
}

// Load reads the configuration from the environment.
//
// The secrets — SESSION_SECRET, LDAP_BIND_PASS, DATABASE_URL and MAIL_PASSWORD —
// can also be read from a file named by the same variable with a _FILE suffix,
// the convention Docker and Kubernetes secrets are mounted for. The error lists
// every such file that could not be used: a secret the operator pointed at a
// file and that silently fell back to its in-repo default would be worse than a
// refusal to start.
func Load() (*Config, error) {
	var l loader
	cfg := &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8000),
		},
		Database: DatabaseConfig{
			URL: l.secret("DATABASE_URL", "postgres://ldapwarden:ldapwarden@localhost:5432/ldapwarden?sslmode=disable"),
		},
		LDAP: LDAPConfig{
			URL:             getEnv("LDAP_URL", "ldap://localhost:389"),
			BaseDN:          getEnv("LDAP_BASE_DN", "dc=example,dc=org"),
			BindDN:          getEnv("LDAP_BIND_DN", "cn=admin,dc=example,dc=org"),
			BindPass:        l.secret("LDAP_BIND_PASS", defaultLDAPBindPass),
			UserOU:          getEnv("LDAP_USER_OU", "ou=People"),
			GroupOU:         getEnv("LDAP_GROUP_OU", "ou=Groups"),
			SudoersOU:       getEnv("LDAP_SUDOERS_OU", "ou=sudoers"),
			PpolicyOU:       getEnv("LDAP_PPOLICY_OU", "ou=policies"),
			NetgroupOU:      getEnv("LDAP_NETGROUP_OU", "ou=netgroups"),
			DefaultPolicyDN: getEnv("LDAP_DEFAULT_POLICY_DN", ""),
			MinUID:          getEnvInt("LDAP_MIN_UID", 1000),
			MinGID:          getEnvInt("LDAP_MIN_GID", 1000),
			TLSMode:         getEnv("LDAP_TLS_MODE", "none"),
			TLSSkipVerify:   getEnvBool("LDAP_TLS_SKIP_VERIFY", false),
			TLSCAFile:       getEnv("LDAP_TLS_CA_FILE", ""),
			TLSCertFile:     getEnv("LDAP_TLS_CERT_FILE", ""),
			TLSKeyFile:      getEnv("LDAP_TLS_KEY_FILE", ""),
			SearchSizeLimit: getEnvInt("LDAP_SEARCH_SIZE_LIMIT", 500),
			Timeout:         getEnvDuration("LDAP_TIMEOUT", 20*time.Second),
		},
		Session: SessionConfig{
			Secret: l.secret("SESSION_SECRET", defaultSessionSecret),
			TTL:    getEnvDuration("SESSION_TTL", 24*time.Hour),
		},
		App: AppConfig{
			AdminGroup:        getEnv("LDAPWARDEN_ADMIN_GROUP", "admins"),
			Organization:      getEnv("LDAPWARDEN_ORGANIZATION", "Example Organization"),
			PublicURL:         getEnv("LDAPWARDEN_PUBLIC_URL", "http://localhost:8000"),
			Modules:           getEnvStringSlice("LDAPWARDEN_MODULES", []string{"users", "groups", "sudo", "policies", "netgroups"}),
			UsersObjects:      getEnvStringSlice("LDAPWARDEN_USERS_OBJECTS", []string{"inetOrgPerson", "posixAccount", "ldapPublicKey", "shadowAccount"}),
			GroupsObjects:     getEnvStringSlice("LDAPWARDEN_GROUPS_OBJECTS", []string{"posixGroup"}),
			AuditNotifyEmails: getEnvStringSlice("LDAPWARDEN_AUDIT_NOTIFY_EMAILS", nil),
			TrustedProxies:    getEnvStringSlice("LDAPWARDEN_TRUSTED_PROXIES", nil),
			CORSOrigins:       getEnvStringSlice("LDAPWARDEN_CORS_ORIGINS", []string{"http://localhost:5173", "http://localhost:3000"}),
			DevMode:           getEnvBool("LDAPWARDEN_DEV_MODE", false),
		},
		Mail: MailConfig{
			Host:     getEnv("MAIL_HOST", "localhost"),
			Port:     getEnvInt("MAIL_PORT", 1025),
			User:     getEnv("MAIL_USER", ""),
			Password: l.secret("MAIL_PASSWORD", ""),
			From:     getEnv("MAIL_FROM", "noreply@example.org"),
			FromName: getEnv("MAIL_FROM_NAME", ""),
			SSL:      getEnv("MAIL_SSL", "none"),
		},
		ScheduledTasks: ScheduledTasksConfig{
			UsersExpiration:     getEnv("LDAPWARDEN_SCHEDULED_TASKS_USERS_EXPIRATION", "42 3 * * *"),
			PasswordsExpiration: getEnv("LDAPWARDEN_SCHEDULED_TASKS_PASSWORDS_EXPIRATION", "42 3 * * *"),
			TokensCleanup:       getEnv("LDAPWARDEN_SCHEDULED_TASKS_TOKENS_CLEANUP", "17 * * * *"),
			LockoutScan:         getEnv("LDAPWARDEN_SCHEDULED_TASKS_LOCKOUT_SCAN", "*/5 * * * *"),
		},
	}
	return cfg, errors.Join(l.errs...)
}

// SecretFileSuffix turns a variable name into the one naming the file that may
// hold its value instead.
const SecretFileSuffix = "_FILE"

// loader collects what Load could not read, so a deployment with several broken
// secret files hears about all of them in one failed start.
type loader struct {
	errs []error
}

// secret reads key from the environment, or from the file key_FILE names.
//
// Setting both is refused rather than resolved: whichever one won, the other
// would be a value the operator believes is in use and is not. An empty file is
// refused too — for a plain variable empty means "unset, use the default", and a
// mounted secret that came out empty is a broken mount, not a request for the
// in-repo default.
//
// Trailing line breaks are stripped, because nearly every way of writing a file
// (echo, a heredoc, an editor) ends it with one and no secret means to end with
// one. Anything else in the file is kept as is.
func (l *loader) secret(key, defaultValue string) string {
	fileVar := key + SecretFileSuffix
	path := os.Getenv(fileVar)
	if path == "" {
		return getEnv(key, defaultValue)
	}
	if os.Getenv(key) != "" {
		l.errs = append(l.errs, fmt.Errorf("both %s and %s are set; keep one", key, fileVar))
		return defaultValue
	}
	data, err := os.ReadFile(path) //nolint:gosec // The path is the operator's own configuration.
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s: %w", fileVar, err))
		return defaultValue
	}
	value := strings.TrimRight(string(data), "\r\n")
	if value == "" {
		l.errs = append(l.errs, fmt.Errorf("%s: %s is empty", fileVar, path))
		return defaultValue
	}
	return value
}

// ValidateSecrets refuses production startup with the in-repo defaults.
// Returns nil when cfg.App.DevMode is true so the bundled docker-compose
// stack and integration tests keep working with their well-known credentials.
// Errors are aggregated so an operator gets a single boot failure that lists
// every misconfiguration instead of one round-trip per secret.
func ValidateSecrets(cfg *Config) error {
	if cfg.App.DevMode {
		return nil
	}
	var errs []error
	switch {
	case cfg.Session.Secret == "":
		errs = append(errs, errors.New("SESSION_SECRET must be set"))
	case cfg.Session.Secret == defaultSessionSecret:
		errs = append(errs, errors.New("SESSION_SECRET is the in-repo default; pick a fresh value"))
	case len(cfg.Session.Secret) < minSessionSecretLen:
		errs = append(errs, fmt.Errorf("SESSION_SECRET must be at least %d bytes", minSessionSecretLen))
	}
	if cfg.LDAP.BindPass == defaultLDAPBindPass {
		errs = append(errs, errors.New("LDAP_BIND_PASS is the in-repo default 'admin'; pick a fresh value"))
	}
	// PublicURL drives password-reset links that travel in email. Plain
	// http:// would let any on-path attacker steal a reset before the user
	// clicks; require https:// in production.
	if strings.HasPrefix(strings.ToLower(cfg.App.PublicURL), "http://") {
		errs = append(errs, errors.New("LDAPWARDEN_PUBLIC_URL must use https:// outside dev mode (reset links would otherwise travel in cleartext)"))
	}
	// The CORS middleware is mounted with AllowCredentials: true, which the
	// CORS spec forbids in combination with "*". Refusing this at startup
	// avoids a misconfiguration that would either silently void credentials
	// (modern browsers) or, worse, accept them from any origin.
	for _, origin := range cfg.App.CORSOrigins {
		if strings.TrimSpace(origin) == "*" {
			errs = append(errs, errors.New("LDAPWARDEN_CORS_ORIGINS cannot contain '*' because the API serves credentialed requests; list explicit origins instead"))
			break
		}
	}
	// Audit-notification recipients are written verbatim into the To: header.
	// The mailer rejects control characters at send time, but that surfaces only
	// when the first notification fires; validate here so a malformed address is
	// a boot failure the operator sees immediately.
	for _, addr := range cfg.App.AuditNotifyEmails {
		a := strings.TrimSpace(addr)
		if a == "" {
			continue
		}
		if strings.ContainsAny(a, "\r\n\x00") {
			errs = append(errs, fmt.Errorf("LDAPWARDEN_AUDIT_NOTIFY_EMAILS entry %q contains control characters", a))
		} else if !strings.Contains(a, "@") {
			errs = append(errs, fmt.Errorf("LDAPWARDEN_AUDIT_NOTIFY_EMAILS entry %q is not a valid email address", a))
		}
	}
	return errors.Join(errs...)
}

// UsesTLS reports whether connections to the directory are encrypted: an
// explicit TLS mode, or an ldaps:// URL, which the client honours on its own.
func (c LDAPConfig) UsesTLS() bool {
	return c.TLSMode == "ssl" || c.TLSMode == "starttls" || strings.HasPrefix(strings.ToLower(c.URL), "ldaps://")
}

// ValidateTLS refuses an LDAP TLS configuration that would not do what it says.
//
// Unlike ValidateSecrets it applies in dev mode too: these are not demo
// credentials to tolerate but settings that are either honoured or not. A CA
// file on a plaintext connection is the case it exists for — nothing reads it,
// the operator believes the directory's certificate is being checked, and the
// bind password crosses the network in clear.
//
// It checks the shape only; whether the files hold usable PEM is for whoever
// loads them (ldap.TLSConfig), which is also what reports it at startup.
func ValidateTLS(c LDAPConfig) error {
	var errs []error
	switch c.TLSMode {
	case "", "none", "ssl", "starttls":
	default:
		errs = append(errs, fmt.Errorf("LDAP_TLS_MODE %q is not one of none, ssl, starttls", c.TLSMode))
	}
	// go-ldap decides on TLS from the scheme alone, so "ssl" on an ldap:// URL
	// used to connect in clear, and "starttls" on ldaps:// would wrap TLS in TLS.
	ldaps := strings.HasPrefix(strings.ToLower(c.URL), "ldaps://")
	if c.TLSMode == "ssl" && !ldaps {
		errs = append(errs, fmt.Errorf("LDAP_TLS_MODE=ssl needs an ldaps:// LDAP_URL, got %s", c.URL))
	}
	if c.TLSMode == "starttls" && ldaps {
		errs = append(errs, fmt.Errorf("LDAP_TLS_MODE=starttls upgrades a plain connection and needs an ldap:// LDAP_URL, got %s", c.URL))
	}
	if (c.TLSCertFile == "") != (c.TLSKeyFile == "") {
		errs = append(errs, errors.New("LDAP_TLS_CERT_FILE and LDAP_TLS_KEY_FILE go together: set both for a client certificate, or neither"))
	}
	if !c.UsesTLS() {
		for _, setting := range []struct{ name, value string }{
			{"LDAP_TLS_CA_FILE", c.TLSCAFile},
			{"LDAP_TLS_CERT_FILE", c.TLSCertFile},
			{"LDAP_TLS_KEY_FILE", c.TLSKeyFile},
		} {
			if setting.value != "" {
				errs = append(errs, fmt.Errorf("%s is set but the connection is not encrypted (LDAP_TLS_MODE=%s, LDAP_URL=%s): it would be ignored", setting.name, c.TLSMode, c.URL))
			}
		}
	}
	if c.TLSSkipVerify && c.TLSCAFile != "" {
		errs = append(errs, errors.New("LDAP_TLS_SKIP_VERIFY disables the check LDAP_TLS_CA_FILE configures; drop one of them"))
	}
	return errors.Join(errs...)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}

func getEnvStringSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		parts := strings.Split(value, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result
	}
	return defaultValue
}
