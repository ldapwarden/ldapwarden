package ldap

import (
	"fmt"
)

// ErrPosixIDTaken reports that a POSIX number is already in use.
//
// Its own type so the API can answer 409 and name the holder, rather than the
// caller pattern-matching on a message.
type ErrPosixIDTaken struct {
	// Kind is "uid" or "gid".
	Kind   string
	Number int
	// Holder is the uid or cn of the entry already using it.
	Holder string
}

func (e *ErrPosixIDTaken) Error() string {
	return fmt.Sprintf("%s number %d already belongs to %q", e.Kind, e.Number, e.Holder)
}

// allocationAttempts bounds the walk for a free number. It is a backstop against
// a directory that answers inconsistently, not a real limit: the search that
// seeds the walk already skips every number in use, so the first candidate is
// normally free.
const allocationAttempts = 50

// reserveUIDNumber settles which uid number a new account will carry, as late as
// possible before the write.
//
// Nothing here is atomic and nothing can be: LDAP has no transaction, and
// OpenLDAP enforces no uniqueness on uidNumber unless the `unique` overlay is
// configured — which the bundled stack does not do, and which is the only real
// guarantee available. What this removes is the window that actually bites: the
// number a creation form read minutes ago, and the same number appearing twice
// in one import. What remains is the microseconds between this check and the
// add, which only two genuinely simultaneous creations can hit.
//
// A `wanted` of 0 means "allocate one" — the caller had no opinion, so a number
// found taken is stepped over. A number the caller asked for is never silently
// swapped: that is an ErrPosixIDTaken for them to resolve.
func (c *Client) reserveUIDNumber(wanted int) (int, error) {
	if wanted != 0 {
		if holder, err := c.UserByUIDNumber(wanted); err != nil {
			return 0, err
		} else if holder != nil {
			return 0, &ErrPosixIDTaken{Kind: "uid", Number: wanted, Holder: holder.UID}
		}
		return wanted, nil
	}

	candidate, err := c.NextUID()
	if err != nil {
		return 0, err
	}
	for attempt := 0; attempt < allocationAttempts; attempt++ {
		holder, err := c.UserByUIDNumber(candidate)
		if err != nil {
			return 0, err
		}
		if holder == nil {
			return candidate, nil
		}
		candidate++
	}
	return 0, fmt.Errorf("no free uid number found in %d attempts from %d",
		allocationAttempts, candidate-allocationAttempts)
}

// reserveGIDNumber is reserveUIDNumber for groups.
func (c *Client) reserveGIDNumber(wanted int) (int, error) {
	if wanted != 0 {
		if holder, err := c.GroupByGIDNumber(wanted); err != nil {
			return 0, err
		} else if holder != nil {
			return 0, &ErrPosixIDTaken{Kind: "gid", Number: wanted, Holder: holder.CN}
		}
		return wanted, nil
	}

	candidate, err := c.NextGID()
	if err != nil {
		return 0, err
	}
	for attempt := 0; attempt < allocationAttempts; attempt++ {
		holder, err := c.GroupByGIDNumber(candidate)
		if err != nil {
			return 0, err
		}
		if holder == nil {
			return candidate, nil
		}
		candidate++
	}
	return 0, fmt.Errorf("no free gid number found in %d attempts from %d",
		allocationAttempts, candidate-allocationAttempts)
}
