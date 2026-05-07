package api

import (
	"net/http"
	"os"
	"strings"
)

type ConfigResponse struct {
	Server   ServerConfigResponse   `json:"server"`
	Database DatabaseConfigResponse `json:"database"`
	Redis    RedisConfigResponse    `json:"redis"`
	LDAP     LDAPConfigResponse     `json:"ldap"`
	Session  SessionConfigResponse  `json:"session"`
	App      AppConfigResponse      `json:"app"`
	Mail     MailConfigResponse     `json:"mail"`
}

type ServerConfigResponse struct {
	Host        ConfigValue `json:"host"`
	Port        ConfigValue `json:"port"`
}

type DatabaseConfigResponse struct {
	URL ConfigValue `json:"url"`
}

type RedisConfigResponse struct {
	URL ConfigValue `json:"url"`
}

type LDAPConfigResponse struct {
	URL           ConfigValue `json:"url"`
	BaseDN        ConfigValue `json:"baseDn"`
	BindDN        ConfigValue `json:"bindDn"`
	UserOU        ConfigValue `json:"userOu"`
	GroupOU       ConfigValue `json:"groupOu"`
	SudoersOU     ConfigValue `json:"sudoersOu"`
	PpolicyOU     ConfigValue `json:"ppolicyOu"`
	MinUID        ConfigValue `json:"minUid"`
	MinGID        ConfigValue `json:"minGid"`
	TLSMode       ConfigValue `json:"tlsMode"`
	TLSSkipVerify ConfigValue `json:"tlsSkipVerify"`
}

type SessionConfigResponse struct {
	TTL ConfigValue `json:"ttl"`
}

type AppConfigResponse struct {
	AdminGroup        ConfigValue `json:"adminGroup"`
	Organization      ConfigValue `json:"organization"`
	PublicURL         ConfigValue `json:"publicUrl"`
	Modules           ConfigValue `json:"modules"`
	UsersObjects      ConfigValue `json:"usersObjects"`
	GroupsObjects     ConfigValue `json:"groupsObjects"`
	AuditNotifyEmails ConfigValue `json:"auditNotifyEmails"`
}

type MailConfigResponse struct {
	Host     ConfigValue `json:"host"`
	Port     ConfigValue `json:"port"`
	User     ConfigValue `json:"user"`
	Password ConfigValue `json:"password"`
	From     ConfigValue `json:"from"`
	SSL      ConfigValue `json:"ssl"`
}

type ConfigValue struct {
	Value  interface{} `json:"value"`
	Source string      `json:"source"` // "env" or "default"
	EnvVar string      `json:"envVar,omitempty"`
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.config

	response := ConfigResponse{
		Server: ServerConfigResponse{
			Host: getConfigValue("SERVER_HOST", cfg.Server.Host, "0.0.0.0"),
			Port: getConfigValue("SERVER_PORT", cfg.Server.Port, 8000),
		},
		Database: DatabaseConfigResponse{
			URL: getConfigValueMasked("DATABASE_URL", cfg.Database.URL, "postgres://ldapwarden:ldapwarden@localhost:5432/ldapwarden?sslmode=disable"),
		},
		Redis: RedisConfigResponse{
			URL: getConfigValueMasked("REDIS_URL", cfg.Redis.URL, "redis://localhost:6379"),
		},
		LDAP: LDAPConfigResponse{
			URL:           getConfigValue("LDAP_URL", cfg.LDAP.URL, "ldap://localhost:389"),
			BaseDN:        getConfigValue("LDAP_BASE_DN", cfg.LDAP.BaseDN, "dc=example,dc=org"),
			BindDN:        getConfigValue("LDAP_BIND_DN", cfg.LDAP.BindDN, "cn=admin,dc=example,dc=org"),
			UserOU:        getConfigValue("LDAP_USER_OU", cfg.LDAP.UserOU, "ou=People"),
			GroupOU:       getConfigValue("LDAP_GROUP_OU", cfg.LDAP.GroupOU, "ou=Groups"),
			SudoersOU:     getConfigValue("LDAP_SUDOERS_OU", cfg.LDAP.SudoersOU, "ou=sudoers"),
			PpolicyOU:     getConfigValue("LDAP_PPOLICY_OU", cfg.LDAP.PpolicyOU, "ou=policies"),
			MinUID:        getConfigValue("LDAP_MIN_UID", cfg.LDAP.MinUID, 1000),
			MinGID:        getConfigValue("LDAP_MIN_GID", cfg.LDAP.MinGID, 1000),
			TLSMode:       getConfigValue("LDAP_TLS_MODE", cfg.LDAP.TLSMode, "none"),
			TLSSkipVerify: getConfigValue("LDAP_TLS_SKIP_VERIFY", cfg.LDAP.TLSSkipVerify, false),
		},
		Session: SessionConfigResponse{
			TTL: getConfigValue("SESSION_TTL", cfg.Session.TTL.String(), "24h0m0s"),
		},
		App: AppConfigResponse{
			AdminGroup:        getConfigValue("LDAPWARDEN_ADMIN_GROUP", cfg.App.AdminGroup, "admins"),
			Organization:      getConfigValue("LDAPWARDEN_ORGANIZATION", cfg.App.Organization, "Example Organization"),
			PublicURL:         getConfigValue("LDAPWARDEN_PUBLIC_URL", cfg.App.PublicURL, "http://localhost:8000"),
			Modules:           getConfigValue("LDAPWARDEN_MODULES", cfg.App.Modules, []string{"users", "groups", "sudo", "policies"}),
			UsersObjects:      getConfigValue("LDAPWARDEN_USERS_OBJECTS", cfg.App.UsersObjects, []string{"inetOrgPerson", "posixAccount", "ldapPublicKey"}),
			GroupsObjects:     getConfigValue("LDAPWARDEN_GROUPS_OBJECTS", cfg.App.GroupsObjects, []string{"posixGroup"}),
			AuditNotifyEmails: getConfigValue("LDAPWARDEN_AUDIT_NOTIFY_EMAILS", cfg.App.AuditNotifyEmails, []string(nil)),
		},
		Mail: MailConfigResponse{
			Host:     getConfigValue("MAIL_HOST", cfg.Mail.Host, "localhost"),
			Port:     getConfigValue("MAIL_PORT", cfg.Mail.Port, 1025),
			User:     getConfigValue("MAIL_USER", cfg.Mail.User, ""),
			Password: getConfigValueMaskedPassword("MAIL_PASSWORD", cfg.Mail.Password),
			From:     getConfigValue("MAIL_FROM", cfg.Mail.From, "noreply@example.org"),
			SSL:      getConfigValue("MAIL_SSL", cfg.Mail.SSL, "none"),
		},
	}

	writeJSON(w, http.StatusOK, response)
}

func getConfigValue(envVar string, currentValue interface{}, defaultValue interface{}) ConfigValue {
	envValue := os.Getenv(envVar)
	source := "default"
	if envValue != "" {
		source = "env"
	}

	return ConfigValue{
		Value:  currentValue,
		Source: source,
		EnvVar: envVar,
	}
}

func getConfigValueMasked(envVar string, currentValue string, defaultValue string) ConfigValue {
	envValue := os.Getenv(envVar)
	source := "default"
	if envValue != "" {
		source = "env"
	}

	// Mask sensitive parts of connection strings
	maskedValue := maskConnectionString(currentValue)

	return ConfigValue{
		Value:  maskedValue,
		Source: source,
		EnvVar: envVar,
	}
}

func maskConnectionString(connStr string) string {
	// Mask password in connection strings like postgres://user:password@host:port/db
	if strings.Contains(connStr, "://") {
		parts := strings.SplitN(connStr, "://", 2)
		if len(parts) == 2 {
			rest := parts[1]
			// Find @ to separate credentials from host
			if atIdx := strings.Index(rest, "@"); atIdx != -1 {
				credentials := rest[:atIdx]
				hostPart := rest[atIdx:]
				// Mask password if present
				if colonIdx := strings.Index(credentials, ":"); colonIdx != -1 {
					user := credentials[:colonIdx]
					return parts[0] + "://" + user + ":****" + hostPart
				}
			}
		}
	}
	return connStr
}

func getConfigValueMaskedPassword(envVar string, currentValue string) ConfigValue {
	envValue := os.Getenv(envVar)
	source := "default"
	if envValue != "" {
		source = "env"
	}

	maskedValue := ""
	if currentValue != "" {
		maskedValue = "****"
	}

	return ConfigValue{
		Value:  maskedValue,
		Source: source,
		EnvVar: envVar,
	}
}
