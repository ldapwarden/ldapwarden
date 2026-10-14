package ldap

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-ldap/ldap/v3"
)

// The accesslog overlay records every write the directory accepts — whoever made
// it — as an entry under its own suffix. That is what lets the app show changes
// it did not make: an ldapmodify, a provisioning script, another admin tool. Its
// own audit log only knows about the requests that came through it.
//
// Reading it needs two things the app cannot arrange itself: the overlay on the
// server (ldap/config/accesslog.ldif shows how) and read access to the log for
// the identity the app binds as.

var (
	// ErrAccessLogNotConfigured: LDAP_ACCESSLOG_BASE is empty.
	ErrAccessLogNotConfigured = errors.New("LDAP_ACCESSLOG_BASE is not set")
	// ErrAccessLogUnreadable: the directory answered "no such object" at the
	// configured base. That is also what slapd answers when an ACL hides the
	// entry, so the two cannot be told apart — see AccessLogChanges.
	ErrAccessLogUnreadable = errors.New("the access log base cannot be read")
)

// AttributeChange is one directive of a logged modify (or one attribute of a
// logged add): the values added, deleted or put in place.
type AttributeChange struct {
	Attribute string `json:"attribute"`
	// Op is add, delete, replace or increment. A delete or replace with no
	// values removes the whole attribute.
	Op     string   `json:"op"`
	Values []string `json:"values,omitempty"`
	// Redacted: a secret (a password hash, say) whose values are not shown.
	Redacted bool `json:"redacted,omitempty"`
}

// AttributeValues is an attribute's previous values, from reqOld.
type AttributeValues struct {
	Attribute string   `json:"attribute"`
	Values    []string `json:"values,omitempty"`
	Redacted  bool     `json:"redacted,omitempty"`
}

// DirectoryChange is one write the directory recorded.
type DirectoryChange struct {
	// ID is reqStart verbatim: unique per log entry, and the RDN it is stored
	// under.
	ID   string    `json:"id"`
	Time time.Time `json:"time"`
	// Type is add, modify, delete or modrdn.
	Type string `json:"type"`
	DN   string `json:"dn"`
	// Actor is the identity the write was authorised as (reqAuthzID). Writes the
	// server makes on its own — ppolicy recording a failed bind — carry the
	// rootdn here, or nothing.
	Actor   string            `json:"actor"`
	Changes []AttributeChange `json:"changes,omitempty"`
	// Old is what a modify replaced and what a delete removed, when the overlay
	// is configured to keep it (olcAccessLogOld).
	Old          []AttributeValues `json:"old,omitempty"`
	NewRDN       string            `json:"newRdn,omitempty"`
	DeleteOldRDN bool              `json:"deleteOldRdn,omitempty"`
	NewSuperior  string            `json:"newSuperior,omitempty"`
}

// accessLogWriteFilter selects the write records. An overlay logging more than
// writes (olcAccessLogOps: all) also holds a record per bind and search, which
// would bury the changes.
const accessLogWriteFilter = "(|(objectClass=auditAdd)(objectClass=auditModify)(objectClass=auditDelete)(objectClass=auditModRDN))"

var accessLogAttributes = []string{
	"reqStart", "reqType", "reqDN", "reqAuthzID", "reqMod", "reqOld",
	"reqNewRDN", "reqDeleteOldRDN", "reqNewSuperior",
}

// AccessLogBase is the configured base of the access log, empty when unset.
func (c *Client) AccessLogBase() string {
	return c.config.AccessLogBase
}

// AccessLogChanges reads the writes logged between from and to (inclusive; a
// zero to leaves the window open, so a write the directory's clock dates a
// moment ahead of the app's is not missed), newest first, at most limit of them. truncated reports that more were in the
// window: the log has no server-side order the app can ask for, so the window is
// read whole and sorted here, and a window too wide to read is cut rather than
// let grow without bound.
//
// ErrAccessLogUnreadable covers both an access log that does not exist and one
// the bind identity is not allowed to see: slapd answers "no such object" to
// both, deliberately, and nothing readable tells them apart.
func (c *Client) AccessLogChanges(from, to time.Time, limit int) (changes []DirectoryChange, truncated bool, err error) {
	base := c.config.AccessLogBase
	if base == "" {
		return nil, false, ErrAccessLogNotConfigured
	}
	conn, err := c.connect()
	if err != nil {
		return nil, false, err
	}

	// reqStart carries microseconds, so the bounds do too: at whole seconds, a
	// write made during the second "to" falls in names a later instant than it.
	filter := accessLogWriteFilter + "(reqStart>=" + ldap.EscapeFilter(accessLogTime(from)) + ")"
	if !to.IsZero() {
		filter += "(reqStart<=" + ldap.EscapeFilter(accessLogTime(to)) + ")"
	}
	filter = "(&" + filter + ")"

	paging := ldap.NewControlPaging(exportPageSize)
	request := ldap.NewSearchRequest(base, ldap.ScopeSingleLevel, ldap.NeverDerefAliases,
		0, searchTimeLimitSeconds(c.timeout()), false, filter, accessLogAttributes,
		[]ldap.Control{paging})
	for {
		result, err := conn.Search(request)
		if err != nil {
			if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
				return nil, false, fmt.Errorf("%w: %s", ErrAccessLogUnreadable, base)
			}
			return nil, false, fmt.Errorf("search access log: %w", err)
		}
		for _, entry := range result.Entries {
			changes = append(changes, parseAccessLogEntry(entry))
		}
		control, ok := ldap.FindControl(result.Controls, ldap.ControlTypePaging).(*ldap.ControlPaging)
		more := ok && len(control.Cookie) > 0
		if len(changes) > limit {
			truncated = true
			if more {
				// A page size of zero with the cookie abandons the paged search
				// (RFC 2696), instead of leaving its state open on the shared
				// connection.
				paging.PagingSize = 0
				paging.SetCookie(control.Cookie)
				_, _ = conn.Search(request)
			}
			break
		}
		if !more {
			break
		}
		paging.SetCookie(control.Cookie)
	}

	// reqStart sorts as text: fixed width, UTC, and unique per entry.
	sort.Slice(changes, func(i, j int) bool { return changes[i].ID > changes[j].ID })
	if len(changes) > limit {
		changes = changes[:limit]
	}
	return changes, truncated, nil
}

// accessLogTime formats an instant the way reqStart is written.
func accessLogTime(t time.Time) string {
	return t.UTC().Format("20060102150405.000000Z")
}

func parseAccessLogEntry(entry *ldap.Entry) DirectoryChange {
	id := entry.GetAttributeValue("reqStart")
	at, _ := parseGeneralizedTime(id)
	change := DirectoryChange{
		ID:           id,
		Time:         at,
		Type:         strings.ToLower(entry.GetAttributeValue("reqType")),
		DN:           entry.GetAttributeValue("reqDN"),
		Actor:        entry.GetAttributeValue("reqAuthzID"),
		NewRDN:       entry.GetAttributeValue("reqNewRDN"),
		DeleteOldRDN: strings.EqualFold(entry.GetAttributeValue("reqDeleteOldRDN"), "TRUE"),
		NewSuperior:  entry.GetAttributeValue("reqNewSuperior"),
	}
	change.Changes = parseReqMod(entry.GetRawAttributeValues("reqMod"))
	change.Old = parseReqOld(entry.GetRawAttributeValues("reqOld"))
	return change
}

// bookkeepingAttributes are maintained by the server on every write. Shown, they
// would bury the change that was made under five lines nobody asked for.
var bookkeepingAttributes = []string{
	"entrycsn", "entryuuid", "creatorsname", "createtimestamp", "modifiersname",
	"modifytimestamp", "structuralobjectclass", "contextcsn", "pwdchangedtime",
}

// secretAttributes are shown as changed, never with their values: password hashes
// are offline-crackable, and pwdHistory is a list of them.
var secretAttributes = []string{
	"userpassword", "authpassword", "pwdhistory",
	"sambantpassword", "sambalmpassword", "sambapasswordhistory",
}

// attributeName strips options ("jpegPhoto;binary") and folds case, for the
// lists above.
func attributeName(attr string) string {
	name, _, _ := strings.Cut(attr, ";")
	return strings.ToLower(name)
}

var reqModOps = map[byte]string{'+': "add", '-': "delete", '=': "replace", '#': "increment"}

// parseReqMod reads reqMod values, "attr:<op> value" — or "attr:<op>" with no
// value for a delete or replace of the whole attribute — merging consecutive
// values of one directive into a single change.
func parseReqMod(raw [][]byte) []AttributeChange {
	var out []AttributeChange
	for _, value := range raw {
		attr, rest, ok := strings.Cut(string(value), ":")
		if !ok || rest == "" {
			continue
		}
		op, known := reqModOps[rest[0]]
		if !known || slices.Contains(bookkeepingAttributes, attributeName(attr)) {
			continue
		}
		// "attr:-" alone deletes the whole attribute; otherwise a space separates
		// the value.
		text := rest[1:]
		hasValue := text != ""
		v := strings.TrimPrefix(text, " ")

		secret := slices.Contains(secretAttributes, attributeName(attr))
		last := len(out) - 1
		if last < 0 || out[last].Attribute != attr || out[last].Op != op {
			out = append(out, AttributeChange{Attribute: attr, Op: op, Redacted: secret})
			last++
		}
		if hasValue && !secret {
			out[last].Values = append(out[last].Values, displayValue([]byte(v)))
		}
	}
	return out
}

// parseReqOld reads reqOld values, "attr: value", grouped by attribute.
func parseReqOld(raw [][]byte) []AttributeValues {
	var out []AttributeValues
	index := map[string]int{}
	for _, value := range raw {
		attr, v, ok := strings.Cut(string(value), ":")
		if !ok || slices.Contains(bookkeepingAttributes, attributeName(attr)) {
			continue
		}
		v = strings.TrimPrefix(v, " ")
		i, seen := index[attr]
		if !seen {
			i = len(out)
			index[attr] = i
			out = append(out, AttributeValues{Attribute: attr, Redacted: slices.Contains(secretAttributes, attributeName(attr))})
		}
		if !out[i].Redacted {
			out[i].Values = append(out[i].Values, displayValue([]byte(v)))
		}
	}
	return out
}

// displayValue is a value as text, or a placeholder for one that is not: a photo
// has nothing to show in a change list but its size.
func displayValue(v []byte) string {
	if !utf8.Valid(v) || slices.ContainsFunc([]rune(string(v)), func(r rune) bool { return r < 0x20 && r != '\t' }) {
		return fmt.Sprintf("(binary, %d bytes)", len(v))
	}
	return string(v)
}

// policyBookkeeping are the attributes ppolicy writes on its own, during binds:
// the failure timestamps, the lock it sets after pwdMaxFailure and lifts after
// pwdLockoutDuration, the grace logins used.
var policyBookkeeping = []string{"pwdfailuretime", "pwdaccountlockedtime", "pwdgraceusetime", "pwdlastsuccess"}

// IsPolicyBookkeeping reports a write ppolicy made by itself, as a side effect
// of someone binding — a failed attempt, a lockout, its lifting — rather than a
// change anyone requested.
//
// Recognised by content, since the identity it is logged under is the rootdn's,
// the same an administrator's ldapmodify may well use. A lock written by hand
// touches pwdAccountLockedTime alone; ppolicy never does — it locks, and
// unlocks, together with the failure timestamps that caused it.
func (d DirectoryChange) IsPolicyBookkeeping() bool {
	if d.Type != "modify" || len(d.Changes) == 0 {
		return false
	}
	driver := false
	for _, change := range d.Changes {
		name := attributeName(change.Attribute)
		if !slices.Contains(policyBookkeeping, name) {
			return false
		}
		if name != "pwdaccountlockedtime" {
			driver = true
		}
	}
	return driver
}
