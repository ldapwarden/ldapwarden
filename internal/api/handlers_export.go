package api

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	goldap "github.com/go-ldap/ldap/v3"

	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// exportWriteTimeout replaces the server's 15s write timeout for an export, which
// streams for as long as the directory takes to page through. Still bounded, so a
// client that stops reading does not hold the LDAP connection for ever.
const exportWriteTimeout = 30 * time.Minute

// handleExportLDIF streams the directory, or part of it, as LDIF.
//
// It is the most sensitive read the app offers — every entry, password hashes
// included — so it is admin-only and audited before anything is sent; with the
// audit log down it refuses, like a write would.
//
// Query: base (default the configured base DN), scope (sub, one or base; default
// sub), operational (true to add every operational attribute).
func (s *Server) handleExportLDIF(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	base := strings.TrimSpace(q.Get("base"))
	if base == "" {
		base = s.ldapClient.BaseDN()
	}
	if _, err := goldap.ParseDN(base); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("base is not a DN: %v", err))
		return
	}
	scope, err := ldap.ParseExportScope(q.Get("scope"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	operational := q.Get("operational") == "true"

	// Checked before the response starts: once the first byte is out, a failure
	// can only cut the download short, not explain itself.
	exists, err := s.ldapClient.EntryExists(base)
	if err != nil {
		writeServerError(w, r, "export ldif", err)
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, fmt.Sprintf("no entry at %s", base))
		return
	}

	if err := s.auditLogger.Log(r.Context(), audit.ActionDirectoryExport, audit.ResourceDirectory, base,
		map[string]interface{}{"scope": string(scope), "operational": operational}); err != nil {
		writeError(w, http.StatusInternalServerError, "audit unavailable; refusing to export")
		return
	}

	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(exportWriteTimeout)); err != nil {
		// Not fatal: a small export fits in the default timeout. A large one will
		// be cut, and the missing "end of export" line says so.
		log.Printf("export ldif: cannot extend the write deadline: %v", err)
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", exportFilename(base)))
	w.Header().Set("Cache-Control", "no-store")

	stats, err := s.ldapClient.ExportLDIF(w, ldap.ExportOptions{BaseDN: base, Scope: scope, Operational: operational})
	if err != nil {
		// The status line is already sent. Aborting the connection makes the
		// download fail visibly; ending it normally would hand over a truncated
		// file that looks like a complete one.
		//nolint:gosec // G706: the base is printed with %q, the scope is validated.
		log.Printf("export ldif: requestId=%s base=%q failed after %d entries: %v",
			middleware.GetReqID(r.Context()), base, stats.Entries, err)
		panic(http.ErrAbortHandler)
	}
	//nolint:gosec // G706: the base is printed with %q, the scope is validated.
	log.Printf("export ldif: base=%q scope=%s entries=%d orphans=%d", base, scope, stats.Entries, stats.Orphans)
}

// exportFilename names the download after what it holds and when.
func exportFilename(base string) string {
	name := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			return r
		case r == '=' || r == ',' || r == ' ':
			return '_'
		}
		return -1
	}, base)
	return fmt.Sprintf("ldapwarden-%s-%s.ldif", name, time.Now().UTC().Format("20060102T150405Z"))
}
