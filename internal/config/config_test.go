package config

import (
	"strings"
	"testing"
)

func newCfg(secret, bindPass string, dev bool) *Config {
	return &Config{
		Session: SessionConfig{Secret: secret},
		LDAP:    LDAPConfig{BindPass: bindPass},
		App:     AppConfig{DevMode: dev},
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
