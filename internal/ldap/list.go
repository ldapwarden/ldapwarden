package ldap

import (
	"sort"
	"strings"
)

// ListOptions describes what a list endpoint should return: which entries
// match, in what order, and which slice of them.
//
// LDAP has no portable server-side sort or paging (RFC 2891 / VLV are optional
// and commonly disabled), so the directory search is still bounded by
// SearchSizeLimit and the ordering and slicing happen here. The point of doing
// it in the backend rather than the browser is the payload: the client receives
// one page of a light projection instead of every matching entry with every
// attribute.
type ListOptions struct {
	Search string
	// Sort names a field understood by the resource being listed; an unknown
	// or empty value falls back to that resource's natural order.
	Sort string
	Desc bool
	// Offset/Limit slice the sorted result. Limit 0 means "no paging" — used by
	// the pickers, which need every match for a search term rather than a page.
	Limit  int
	Offset int
}

// page returns the requested slice of items, clamped to the available range so
// an out-of-date page number yields an empty page rather than an error.
func page[T any](items []T, opts ListOptions) []T {
	if opts.Limit <= 0 {
		return items
	}
	start := opts.Offset
	if start < 0 {
		start = 0
	}
	if start > len(items) {
		return []T{}
	}
	end := start + opts.Limit
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

// sortBy orders items with a stable sort so entries that compare equal keep the
// directory's order instead of shuffling between pages.
func sortBy[T any](items []T, desc bool, less func(a, b T) bool) {
	sort.SliceStable(items, func(i, j int) bool {
		if desc {
			return less(items[j], items[i])
		}
		return less(items[i], items[j])
	})
}

// foldLess compares strings case-insensitively, which is how the UI presents
// uids, names and mail addresses.
func foldLess(a, b string) bool {
	return strings.ToLower(a) < strings.ToLower(b)
}
