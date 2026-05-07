package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	ldaplib "github.com/go-ldap/ldap/v3"
)

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
