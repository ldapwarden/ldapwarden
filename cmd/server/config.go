package main

import (
	"fmt"

	"github.com/ldapwarden/ldapwarden/internal/config"
	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// loadConfig reads the configuration and refuses one that cannot be honoured:
// a secret file that cannot be read, TLS settings that contradict each other,
// or certificate files that do not load. Every entry point that talks to the
// directory starts here, so a maintenance command fails the same way the server
// would rather than connecting with settings the server would have refused.
//
// The demo-secret check (config.ValidateSecrets) is not part of it: it guards a
// served deployment, and the server applies it on top.
func loadConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("configuration: %w", err)
	}
	if err := config.ValidateTLS(cfg.LDAP); err != nil {
		return nil, fmt.Errorf("LDAP TLS configuration: %w", err)
	}
	if _, err := ldap.TLSConfig(cfg.LDAP); err != nil {
		return nil, fmt.Errorf("LDAP TLS configuration: %w", err)
	}
	return cfg, nil
}
