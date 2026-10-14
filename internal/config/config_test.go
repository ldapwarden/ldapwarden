package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newCfg(secret, bindPass string, dev bool) *Config {
	return &Config{
		Session: SessionConfig{Secret: secret},
		LDAP:    LDAPConfig{BindPass: bindPass},
		// A safe default that ValidateSecrets does not flag, so individual
		// cases below only have to override what they want to test.
		App: AppConfig{PublicURL: "https://ldap.example.org", DevMode: dev},
	}
}

func TestValidateSecrets(t *testing.T) {
	const goodSecret = "this-is-a-completely-different-32+bytes!"

	cases := []struct {
		name      string
		cfg       *Config
		wantErr   bool
		mustMatch []string // substrings that the error message must contain
	}{
		{
			name:    "valid",
			cfg:     newCfg(goodSecret, "real-password", false),
			wantErr: false,
		},
		{
			name:      "empty session secret",
			cfg:       newCfg("", "real-password", false),
			wantErr:   true,
			mustMatch: []string{"SESSION_SECRET must be set"},
		},
		{
			name:      "default session secret",
			cfg:       newCfg(defaultSessionSecret, "real-password", false),
			wantErr:   true,
			mustMatch: []string{"in-repo default"},
		},
		{
			name:      "short session secret",
			cfg:       newCfg("too-short", "real-password", false),
			wantErr:   true,
			mustMatch: []string{"at least 32 bytes"},
		},
		{
			name:      "default bind pass",
			cfg:       newCfg(goodSecret, defaultLDAPBindPass, false),
			wantErr:   true,
			mustMatch: []string{"LDAP_BIND_PASS"},
		},
		{
			name:      "both bad: aggregated error",
			cfg:       newCfg(defaultSessionSecret, defaultLDAPBindPass, false),
			wantErr:   true,
			mustMatch: []string{"SESSION_SECRET", "LDAP_BIND_PASS"},
		},
		{
			name:    "dev mode bypasses every check",
			cfg:     newCfg(defaultSessionSecret, defaultLDAPBindPass, true),
			wantErr: false,
		},
		{
			name: "http public url rejected outside dev",
			cfg: func() *Config {
				c := newCfg(goodSecret, "real-password", false)
				c.App.PublicURL = "http://ldap.example.org"
				return c
			}(),
			wantErr:   true,
			mustMatch: []string{"LDAPWARDEN_PUBLIC_URL", "https://"},
		},
		{
			name: "http public url tolerated in dev",
			cfg: func() *Config {
				c := newCfg(goodSecret, "real-password", true)
				c.App.PublicURL = "http://ldap.example.org"
				return c
			}(),
			wantErr: false,
		},
		{
			name: "cors wildcard rejected with credentials",
			cfg: func() *Config {
				c := newCfg(goodSecret, "real-password", false)
				c.App.CORSOrigins = []string{"https://app.example.org", "*"}
				return c
			}(),
			wantErr:   true,
			mustMatch: []string{"LDAPWARDEN_CORS_ORIGINS", "*"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSecrets(tc.cfg)
			if tc.wantErr && err == nil {
				t.Fatalf("want error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("want nil, got %v", err)
			}
			if err != nil {
				msg := err.Error()
				for _, m := range tc.mustMatch {
					if !strings.Contains(msg, m) {
						t.Errorf("error %q missing %q", msg, m)
					}
				}
			}
		})
	}
}

// TestValidateSecrets_ConstMatchesDefault guards against future drift between
// the default in Load() and the constant rejected by ValidateSecrets. If
// someone bumps the default they must bump the constant too, otherwise this
// fix becomes a no-op.
func TestValidateSecrets_ConstMatchesDefault(t *testing.T) {
	t.Setenv("SESSION_SECRET", "")
	t.Setenv("LDAP_BIND_PASS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Session.Secret != defaultSessionSecret {
		t.Errorf("Load() produced session secret %q, want default %q", cfg.Session.Secret, defaultSessionSecret)
	}
	if cfg.LDAP.BindPass != defaultLDAPBindPass {
		t.Errorf("Load() produced bind pass %q, want default %q", cfg.LDAP.BindPass, defaultLDAPBindPass)
	}
}

// TestDefaultModules pins the module list a deployment gets when
// LDAPWARDEN_MODULES is unset.
//
// The frontend carries the same list, as DEFAULT_MODULES in
// web/src/lib/netgroup.ts, for the sessions that cannot read the config
// endpoint. The two drifted apart once — netgroups was added here and the
// frontend copies were left behind, so a non-admin lost a tab on a server that
// had it enabled, silently. Both sides are pinned so changing one fails until
// the other follows.
func TestDefaultModules(t *testing.T) {
	t.Setenv("LDAPWARDEN_MODULES", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"users", "groups", "sudo", "policies", "netgroups"}
	if len(cfg.App.Modules) != len(want) {
		t.Fatalf("default modules = %v, want %v", cfg.App.Modules, want)
	}
	for i, module := range want {
		if cfg.App.Modules[i] != module {
			t.Errorf("default modules = %v, want %v", cfg.App.Modules, want)
			break
		}
	}
}

func writeSecret(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_SecretFromFile(t *testing.T) {
	t.Setenv("LDAP_BIND_PASS", "")
	t.Setenv("LDAP_BIND_PASS_FILE", writeSecret(t, "s3cret with spaces \n"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// The trailing newline every editor adds goes; the rest is the secret.
	if cfg.LDAP.BindPass != "s3cret with spaces " {
		t.Errorf("bind pass = %q, want %q", cfg.LDAP.BindPass, "s3cret with spaces ")
	}
}

func TestLoad_SecretFileErrors(t *testing.T) {
	cases := []struct {
		name      string
		setup     func(t *testing.T)
		mustMatch []string
	}{
		{
			name: "both the variable and its file",
			setup: func(t *testing.T) {
				t.Setenv("SESSION_SECRET", "inline")
				t.Setenv("SESSION_SECRET_FILE", writeSecret(t, "from-file"))
			},
			mustMatch: []string{"SESSION_SECRET and SESSION_SECRET_FILE"},
		},
		{
			name: "missing file",
			setup: func(t *testing.T) {
				t.Setenv("DATABASE_URL_FILE", filepath.Join(t.TempDir(), "absent"))
			},
			mustMatch: []string{"DATABASE_URL_FILE"},
		},
		{
			// An empty mount is a broken mount: falling back to the in-repo
			// default would hide it.
			name: "empty file",
			setup: func(t *testing.T) {
				t.Setenv("MAIL_PASSWORD_FILE", writeSecret(t, "\n"))
			},
			mustMatch: []string{"MAIL_PASSWORD_FILE", "is empty"},
		},
		{
			name: "every failure reported at once",
			setup: func(t *testing.T) {
				t.Setenv("LDAP_BIND_PASS_FILE", filepath.Join(t.TempDir(), "absent"))
				t.Setenv("MAIL_PASSWORD_FILE", writeSecret(t, ""))
			},
			mustMatch: []string{"LDAP_BIND_PASS_FILE", "MAIL_PASSWORD_FILE"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range []string{"SESSION_SECRET", "LDAP_BIND_PASS", "DATABASE_URL", "MAIL_PASSWORD"} {
				t.Setenv(key, "")
				t.Setenv(key+SecretFileSuffix, "")
			}
			tc.setup(t)
			_, err := Load()
			if err == nil {
				t.Fatal("want error, got nil")
			}
			for _, m := range tc.mustMatch {
				if !strings.Contains(err.Error(), m) {
					t.Errorf("error %q missing %q", err, m)
				}
			}
		})
	}
}

func TestValidateTLS(t *testing.T) {
	cases := []struct {
		name      string
		cfg       LDAPConfig
		mustMatch []string // empty: valid
	}{
		{name: "plain", cfg: LDAPConfig{URL: "ldap://dir:389", TLSMode: "none"}},
		{name: "ldaps url without mode", cfg: LDAPConfig{URL: "ldaps://dir", TLSMode: "none", TLSCAFile: "/ca.pem"}},
		{name: "ssl with ca and client cert", cfg: LDAPConfig{URL: "ldaps://dir", TLSMode: "ssl", TLSCAFile: "/ca.pem", TLSCertFile: "/c.pem", TLSKeyFile: "/k.pem"}},
		{name: "starttls", cfg: LDAPConfig{URL: "ldap://dir", TLSMode: "starttls", TLSCAFile: "/ca.pem"}},
		{
			name:      "unknown mode",
			cfg:       LDAPConfig{URL: "ldap://dir", TLSMode: "tls"},
			mustMatch: []string{`LDAP_TLS_MODE "tls"`},
		},
		{
			// go-ldap only encrypts the ldaps:// scheme: this connected in clear.
			name:      "ssl on an ldap url",
			cfg:       LDAPConfig{URL: "ldap://dir:636", TLSMode: "ssl"},
			mustMatch: []string{"needs an ldaps:// LDAP_URL"},
		},
		{
			name:      "starttls on an ldaps url",
			cfg:       LDAPConfig{URL: "ldaps://dir", TLSMode: "starttls"},
			mustMatch: []string{"needs an ldap:// LDAP_URL"},
		},
		{
			name:      "ca file on a plaintext connection",
			cfg:       LDAPConfig{URL: "ldap://dir", TLSMode: "none", TLSCAFile: "/ca.pem"},
			mustMatch: []string{"LDAP_TLS_CA_FILE is set but the connection is not encrypted"},
		},
		{
			name:      "certificate without key",
			cfg:       LDAPConfig{URL: "ldaps://dir", TLSMode: "ssl", TLSCertFile: "/c.pem"},
			mustMatch: []string{"go together"},
		},
		{
			name:      "skip verify cancels the ca",
			cfg:       LDAPConfig{URL: "ldaps://dir", TLSMode: "ssl", TLSCAFile: "/ca.pem", TLSSkipVerify: true},
			mustMatch: []string{"LDAP_TLS_SKIP_VERIFY"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTLS(tc.cfg)
			if len(tc.mustMatch) == 0 {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("want error, got nil")
			}
			for _, m := range tc.mustMatch {
				if !strings.Contains(err.Error(), m) {
					t.Errorf("error %q missing %q", err, m)
				}
			}
		})
	}
}
