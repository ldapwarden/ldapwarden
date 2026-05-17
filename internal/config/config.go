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
	Redis          RedisConfig
	LDAP           LDAPConfig
	Session        SessionConfig
	App            AppConfig
	Mail           MailConfig
	ScheduledTasks ScheduledTasksConfig
}

type ScheduledTasksConfig struct {
	UsersExpiration     string // Cron format, empty = disabled
	PasswordsExpiration string // Cron format, empty = disabled
	TokensCleanup       string // Cron format, empty = disabled. Purges expired password_reset_tokens rows.
}

type AppConfig struct {
	AdminGroup        string
	Organization      string
	PublicURL         string
	Modules           []string // High-level modules: users, groups, sudo, policies
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
	SSL      string // "none", "starttls", "ssl"
}

type ServerConfig struct {
	Host string
	Port int
}

type DatabaseConfig struct {
	URL string
}

type RedisConfig struct {
	URL string
}

type LDAPConfig struct {
	URL               string
	BaseDN            string
	BindDN            string
	BindPass          string
	UserOU            string
	GroupOU           string
	SudoersOU         string
	PpolicyOU         string
	MinUID            int
	MinGID            int
	TLSMode           string // "none", "ssl", "starttls"
	TLSSkipVerify     bool   // Skip certificate verification (for self-signed certs)
}

type SessionConfig struct {
	Secret string
	TTL    time.Duration
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8000),
		},
		Database: DatabaseConfig{
			URL: getEnv("DATABASE_URL", "postgres://ldapwarden:ldapwarden@localhost:5433/ldapwarden?sslmode=disable"),
		},
		Redis: RedisConfig{
			URL: getEnv("REDIS_URL", "redis://localhost:6380"),
		},
		LDAP: LDAPConfig{
			URL:           getEnv("LDAP_URL", "ldap://localhost:389"),
			BaseDN:        getEnv("LDAP_BASE_DN", "dc=example,dc=org"),
			BindDN:        getEnv("LDAP_BIND_DN", "cn=admin,dc=example,dc=org"),
			BindPass:      getEnv("LDAP_BIND_PASS", defaultLDAPBindPass),
			UserOU:        getEnv("LDAP_USER_OU", "ou=People"),
			GroupOU:       getEnv("LDAP_GROUP_OU", "ou=Groups"),
			SudoersOU:     getEnv("LDAP_SUDOERS_OU", "ou=sudoers"),
			PpolicyOU:     getEnv("LDAP_PPOLICY_OU", "ou=policies"),
			MinUID:        getEnvInt("LDAP_MIN_UID", 1000),
			MinGID:        getEnvInt("LDAP_MIN_GID", 1000),
			TLSMode:       getEnv("LDAP_TLS_MODE", "none"),
			TLSSkipVerify: getEnvBool("LDAP_TLS_SKIP_VERIFY", false),
		},
		Session: SessionConfig{
			Secret: getEnv("SESSION_SECRET", defaultSessionSecret),
			TTL:    getEnvDuration("SESSION_TTL", 24*time.Hour),
		},
		App: AppConfig{
			AdminGroup:        getEnv("LDAPWARDEN_ADMIN_GROUP", "admins"),
			Organization:      getEnv("LDAPWARDEN_ORGANIZATION", "Example Organization"),
			PublicURL:         getEnv("LDAPWARDEN_PUBLIC_URL", "http://localhost:8000"),
			Modules:           getEnvStringSlice("LDAPWARDEN_MODULES", []string{"users", "groups", "sudo", "policies"}),
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
			Password: getEnv("MAIL_PASSWORD", ""),
			From:     getEnv("MAIL_FROM", "noreply@example.org"),
			SSL:      getEnv("MAIL_SSL", "none"),
		},
		ScheduledTasks: ScheduledTasksConfig{
			UsersExpiration:     getEnv("LDAPWARDEN_SCHEDULED_TASKS_USERS_EXPIRATION", "42 3 * * *"),
			PasswordsExpiration: getEnv("LDAPWARDEN_SCHEDULED_TASKS_PASSWORDS_EXPIRATION", "42 3 * * *"),
			TokensCleanup:       getEnv("LDAPWARDEN_SCHEDULED_TASKS_TOKENS_CLEANUP", "17 * * * *"),
		},
	}
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
