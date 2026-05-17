package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	"github.com/go-chi/chi/v5"
	ldaplib "github.com/go-ldap/ldap/v3"
)

// rdnValuePattern restricts the value side of a UID/CN RDN at create time to
// a conservative POSIX-friendly identifier set. The goal is not to be RFC-4514
// compliant — the LDAP layer also calls ldap.EscapeDN for defense in depth —
// but to refuse anything that could shift the apparent OU or break downstream
// tooling that relies on bare uid/cn (shells, sudo, audit log resource DNs).
var rdnValuePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// validateRDNValue is called at the API boundary whenever a client-supplied
// UID or CN will become the value of an RDN. Returns a non-nil error when the
// value is empty or contains characters outside rdnValuePattern.
func validateRDNValue(field, value string) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	if !rdnValuePattern.MatchString(value) {
		return fmt.Errorf("%s must contain only letters, digits, '.', '_' or '-'", field)
	}
	return nil
}

// posixIDMax is the inclusive upper bound for a POSIX uid_t / gid_t. Most
// kernels treat values at or above 2^31 as reserved (the sign bit catches
// `(uid_t)-1` / "no change"), and overflowing into negative space breaks
// shell tools that store the ID in a signed int. We refuse anything above.
const posixIDMax = (1 << 31) - 1

// validatePOSIXID rejects uidNumber/gidNumber values that fall below the
// configured floor (admin-reserved range) or above the signed-32-bit POSIX
// ceiling. Without this an admin could mint a 0 (= root) or a 2_147_483_648
// account that breaks `id`, NSS, and any consumer that assumes int32.
func validatePOSIXID(field string, value, min int) error {
	if value < min {
		return fmt.Errorf("%s must be >= %d", field, min)
	}
	if value > posixIDMax {
		return fmt.Errorf("%s must be <= %d", field, posixIDMax)
	}
	return nil
}

// errInvalidDN is the generic error surfaced to clients when the {dn} URL
// parameter cannot be parsed or falls outside the allowed scope. The exact
// reason is intentionally not echoed back to avoid giving probing tools a
// distinguishing oracle.
var errInvalidDN = errors.New("invalid dn")

// resolveDN extracts the chi {dn} URL parameter, percent-decodes it, parses
// it as an LDAP DN, and verifies that it is strictly under the supplied
// base DN. Returns the decoded DN string on success.
//
// The scope check uses AncestorOfFold so that the comparison honours LDAP
// distinguishedNameMatch semantics (case-insensitive on the attribute type).
// DNs that equal the base itself are rejected — handlers manage entries
// inside the base, never the base entry.
func resolveDN(r *http.Request, base string) (string, error) {
	raw := chi.URLParam(r, "dn")
	if raw == "" {
		return "", errInvalidDN
	}

	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return "", errInvalidDN
	}

	parsed, err := ldaplib.ParseDN(decoded)
	if err != nil {
		return "", errInvalidDN
	}

	baseParsed, err := ldaplib.ParseDN(base)
	if err != nil {
		// A misconfigured base is an operator-side bug, not a client error.
		return "", fmt.Errorf("server misconfigured: invalid base dn %q: %w", base, err)
	}

	if !baseParsed.AncestorOfFold(parsed) {
		return "", errInvalidDN
	}

	return decoded, nil
}

// dnFirstRDNValue returns the value of the first RDN of dn — i.e. "alice" for
// "uid=alice,ou=People,dc=example,dc=org" or "admins" for "cn=admins,...".
// Returns "" when the DN cannot be parsed or has no RDN.
func dnFirstRDNValue(dn string) string {
	parsed, err := ldaplib.ParseDN(dn)
	if err != nil || len(parsed.RDNs) == 0 || len(parsed.RDNs[0].Attributes) == 0 {
		return ""
	}
	return parsed.RDNs[0].Attributes[0].Value
}
