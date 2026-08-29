package config

import (
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
	cfg := Load()
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

	cfg := Load()
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
