package api

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// Where a logged write came from.
const (
	// originApp: a write this app made, matched to its audit row.
	originApp = "ldapwarden"
	// originPolicy: ppolicy's own bookkeeping during a bind — failed attempts,
	// the lockout they cause, its lifting.
	originPolicy = "policy"
	// originExternal: everything else — the writes nothing in this app records.
	originExternal = "external"
)

const (
	// accessLogReadLimit bounds the writes read for one window. The log has no
	// order the app can request, so a window is read whole and sorted here.
	accessLogReadLimit = 5000

	// An audit row is written just before the request's first directory write,
	// and the writes that follow it — a rename rewriting memberUid in every group
	// — come over the next few seconds. The two timestamps also come from two
	// clocks, the app host's and the directory's, hence some slack both ways.
	auditMatchBefore = 30 * time.Second
	auditMatchAfter  = 5 * time.Second
)

type directoryChangeItem struct {
	ldap.DirectoryChange
	Origin string `json:"origin"`
	// AuditLogID is the audit row the write was matched to, for origin
	// "ldapwarden".
	AuditLogID string `json:"auditLogId,omitempty"`
}

type directoryChangesResponse struct {
	// Status is "ok", "not-configured" (LDAP_ACCESSLOG_BASE unset) or
	// "unreadable" (no such object at the base: absent, or hidden by an ACL).
	Status string `json:"status"`
	Base   string `json:"base,omitempty"`
	// BindDN is the identity that needs read access to the log, for the
	// instructions shown when it cannot read it.
	BindDN string                `json:"bindDn"`
	From   time.Time             `json:"from"`
	To     time.Time             `json:"to"`
	Data   []directoryChangeItem `json:"data"`
	Total  int                   `json:"total"`
	Limit  int                   `json:"limit"`
	Offset int                   `json:"offset"`
	// Truncated: the window held more than accessLogReadLimit writes, and only
	// the newest were examined.
	Truncated bool `json:"truncated"`
	// Counts per origin across the window, before the origin filter, so the
	// view can say how many it is not showing.
	Counts map[string]int `json:"counts"`
}

// handleListDirectoryChanges lists the writes the directory itself logged
// (accesslog overlay), each labelled with where it came from.
//
// Query: from, to (days, YYYY-MM-DD; default the last 24 hours), origin
// (external — the default —, ldapwarden, policy or all), type (add, modify,
// delete, modrdn), search (substring of the target DN or the actor), limit,
// offset.
func (s *Server) handleListDirectoryChanges(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	// Without an explicit end the window stays open: "now" is the app's clock, and
	// reqStart the directory's.
	var to time.Time
	if day, err := time.Parse(time.DateOnly, q.Get("to")); err == nil {
		to = day.Add(24*time.Hour - time.Nanosecond)
	}
	end := to
	if end.IsZero() {
		end = time.Now().UTC()
	}
	from := end.Add(-24 * time.Hour)
	if day, err := time.Parse(time.DateOnly, q.Get("from")); err == nil {
		from = day
	}
	if from.After(end) {
		writeError(w, http.StatusBadRequest, "from is after to")
		return
	}

	origin := q.Get("origin")
	switch origin {
	case "":
		origin = originExternal
	case originExternal, originApp, originPolicy, "all":
	default:
		writeError(w, http.StatusBadRequest, "origin must be one of external, ldapwarden, policy, all")
		return
	}
	limit := 50
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 && n <= 100 {
		limit = n
	}
	offset := 0
	if n, err := strconv.Atoi(q.Get("offset")); err == nil && n > 0 {
		offset = n
	}

	response := directoryChangesResponse{
		Status: "ok",
		Base:   s.ldapClient.AccessLogBase(),
		BindDN: s.config.LDAP.BindDN,
		From:   from,
		To:     end,
		Data:   []directoryChangeItem{},
		Limit:  limit,
		Offset: offset,
		Counts: map[string]int{originExternal: 0, originApp: 0, originPolicy: 0},
	}

	changes, truncated, err := s.ldapClient.AccessLogChanges(from, to, accessLogReadLimit)
	switch {
	case errors.Is(err, ldap.ErrAccessLogNotConfigured):
		response.Status = "not-configured"
		writeJSON(w, http.StatusOK, response)
		return
	case errors.Is(err, ldap.ErrAccessLogUnreadable):
		response.Status = "unreadable"
		writeJSON(w, http.StatusOK, response)
		return
	case err != nil:
		writeServerError(w, r, "list directory changes", err)
		return
	}
	response.Truncated = truncated

	var writes []audit.DirectoryWrite
	if len(changes) > 0 {
		// changes is newest first.
		oldest, newest := changes[len(changes)-1].Time, changes[0].Time
		writes, err = s.auditLogger.DirectoryWritesBetween(r.Context(),
			oldest.Add(-auditMatchBefore), newest.Add(auditMatchAfter))
		if err != nil {
			writeServerError(w, r, "list directory changes", err)
			return
		}
	}

	kind := strings.ToLower(q.Get("type"))
	search := strings.ToLower(strings.TrimSpace(q.Get("search")))
	var matched []directoryChangeItem
	for _, change := range changes {
		item := s.classifyChange(change, writes)
		response.Counts[item.Origin]++
		if origin != "all" && item.Origin != origin {
			continue
		}
		if kind != "" && change.Type != kind {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(change.DN), search) &&
			!strings.Contains(strings.ToLower(change.Actor), search) {
			continue
		}
		matched = append(matched, item)
	}

	response.Total = len(matched)
	if offset < len(matched) {
		end := min(offset+limit, len(matched))
		response.Data = matched[offset:end]
	}
	writeJSON(w, http.StatusOK, response)
}

// classifyChange says where a logged write came from.
//
// The identity alone cannot: the app commonly binds as the rootdn, which is also
// what an administrator's ldapmodify uses and what ppolicy's own writes are
// logged under. So a write counts as the app's only when the app recorded a
// request that writes, by that identity — its bind DN, or the user a self-service
// change was made for — in the moments around it. A write made with the app's
// credentials while the app itself was idle reads as external, which is what it
// is.
func (s *Server) classifyChange(change ldap.DirectoryChange, writes []audit.DirectoryWrite) directoryChangeItem {
	item := directoryChangeItem{DirectoryChange: change, Origin: originExternal}
	if change.IsPolicyBookkeeping() {
		item.Origin = originPolicy
		return item
	}
	if change.Time.IsZero() {
		return item
	}

	// An empty reqAuthzID — an anonymous or internal write — matches nobody,
	// though an empty DN parses.
	if change.Actor == "" {
		return item
	}
	byApp := sameDN(change.Actor, s.config.LDAP.BindDN)
	// writes is oldest first: find the first one inside the window.
	start := sort.Search(len(writes), func(i int) bool {
		return !writes[i].CreatedAt.Before(change.Time.Add(-auditMatchBefore))
	})
	for _, write := range writes[start:] {
		if write.CreatedAt.After(change.Time.Add(auditMatchAfter)) {
			break
		}
		if byApp || sameDN(change.Actor, write.ActorDN) {
			item.Origin = originApp
			item.AuditLogID = write.ID
			return item
		}
	}
	return item
}
