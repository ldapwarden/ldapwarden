package ldap

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/ldapwarden/ldapwarden/internal/config"
)

// TLSConfig builds the TLS settings for connections to the directory.
//
// The files are read on every call, which is every new connection: a rotated
// client certificate or CA bundle is picked up at the next reconnect instead of
// waiting for a restart that nothing prompts. Startup calls it once as well, so
// a file that cannot be used stops the server instead of the first request.
//
// ServerName is filled in from the URL. go-ldap leaves it empty and Go refuses a
// verifying handshake without one, so StartTLS with verification on could never
// connect — the only way to make it work used to be LDAP_TLS_SKIP_VERIFY.
func TLSConfig(cfg config.LDAPConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: cfg.TLSSkipVerify, //nolint:gosec // User-configurable setting
	}

	if u, err := url.Parse(cfg.URL); err == nil {
		host := u.Host
		if h, _, splitErr := net.SplitHostPort(u.Host); splitErr == nil {
			host = h
		}
		tlsConfig.ServerName = host
	}

	if cfg.TLSCAFile != "" {
		pem, err := os.ReadFile(cfg.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("LDAP_TLS_CA_FILE: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("LDAP_TLS_CA_FILE: no PEM certificate in %s", cfg.TLSCAFile)
		}
		tlsConfig.RootCAs = pool
	}

	if cfg.TLSCertFile != "" || cfg.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("LDAP_TLS_CERT_FILE / LDAP_TLS_KEY_FILE: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

// describeTLS is appended to a failed connection's error: which TLS settings were
// in play. A certificate error names neither the CA it was checked against nor
// whether checking was on, and those are the two things the operator has to
// change. Empty for a plaintext connection, where none of it applies.
func describeTLS(cfg config.LDAPConfig) string {
	if !cfg.UsesTLS() {
		return ""
	}
	mode := cfg.TLSMode
	if mode == "" || mode == "none" {
		mode = "ssl (from the ldaps:// URL)"
	}
	parts := []string{"TLS mode " + mode}
	switch {
	case cfg.TLSSkipVerify:
		parts = append(parts, "certificate verification OFF")
	case cfg.TLSCAFile != "":
		parts = append(parts, "CA "+cfg.TLSCAFile)
	default:
		parts = append(parts, "system CAs")
	}
	if cfg.TLSCertFile != "" {
		parts = append(parts, "client certificate "+cfg.TLSCertFile)
	}
	return " [" + strings.Join(parts, ", ") + "]"
}

// errTLSSchemeMismatch is returned when the mode and the URL disagree on whether
// TLS starts with the connection. config.ValidateTLS refuses this at startup;
// the client checks again so a configuration built some other way cannot
// silently connect in clear.
var errTLSSchemeMismatch = errors.New("LDAP TLS mode does not match the URL scheme")
