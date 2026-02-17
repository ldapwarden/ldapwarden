package config

import (
	"os"
	"strconv"
	"strings"
	"time"
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
}

type AppConfig struct {
	AdminGroup    string
	Organization  string
	PublicURL     string
	Modules       []string // High-level modules: users, groups, sudo, policies
	UsersObjects  []string // LDAP objectClasses for users
	GroupsObjects []string // LDAP objectClasses for groups
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
			BindPass:      getEnv("LDAP_BIND_PASS", "admin"),
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
			Secret: getEnv("SESSION_SECRET", "change-me-in-production-32bytes!"),
			TTL:    getEnvDuration("SESSION_TTL", 24*time.Hour),
		},
		App: AppConfig{
			AdminGroup:    getEnv("LDAPWARDEN_ADMIN_GROUP", "admins"),
			Organization:  getEnv("LDAPWARDEN_ORGANIZATION", "Example Organization"),
			PublicURL:     getEnv("LDAPWARDEN_PUBLIC_URL", "http://localhost:8000"),
			Modules:       getEnvStringSlice("LDAPWARDEN_MODULES", []string{"users", "groups", "sudo", "policies"}),
			UsersObjects:  getEnvStringSlice("LDAPWARDEN_USERS_OBJECTS", []string{"inetOrgPerson", "posixAccount", "ldapPublicKey", "shadowAccount"}),
			GroupsObjects: getEnvStringSlice("LDAPWARDEN_GROUPS_OBJECTS", []string{"posixGroup"}),
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
		},
	}
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
