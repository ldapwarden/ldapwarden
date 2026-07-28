package api

import (
	"net/http"
	"strconv"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// maxListLimit caps how large a page a client may ask for, so a hand-crafted
// `?limit=100000` cannot turn a paged endpoint back into a full dump.
const maxListLimit = 500

// parseListOptions reads the paging and sorting parameters shared by the list
// endpoints: search, sort, order, limit, offset.
//
// A missing or zero limit means "no paging": the pickers in the UI need every
// entry matching a search term, and the directory-side size limit already
// bounds that. Only the list views pass an explicit limit.
func parseListOptions(r *http.Request) ldap.ListOptions {
	q := r.URL.Query()

	limit := clampedAtoi(q.Get("limit"), 0, maxListLimit)
	offset := clampedAtoi(q.Get("offset"), 0, 0)

	return ldap.ListOptions{
		Search: q.Get("search"),
		Sort:   q.Get("sort"),
		Desc:   q.Get("order") == "desc",
		Limit:  limit,
		Offset: offset,
	}
}

// clampedAtoi parses a non-negative integer, falling back to fallback on a
// malformed or negative value, and capping it at max when max > 0.
func clampedAtoi(raw string, fallback, max int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return fallback
	}
	if max > 0 && n > max {
		return max
	}
	return n
}
