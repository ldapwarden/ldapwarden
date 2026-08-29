package ldap

import (
	"fmt"
	"sort"
)

// NetgroupMember is one name a netgroup contributes, with where it came from.
//
// Via is empty when the netgroup named it directly, and otherwise holds the
// nested netgroup whose triple did. It exists because "jdoe is in ops-oncall"
// and "jdoe is in ops-oncall via ops" call for different actions: the first is
// edited here, the second somewhere else.
type NetgroupMember struct {
	Name string `json:"name"`
	Via  string `json:"via,omitempty"`
}

// NetgroupExpansion is what a netgroup contributes once nesting is followed.
//
// Deliberately not called "members". A triple couples a host to a user —
// "(web01,jdoe,)" means jdoe *on* web01 — and splitting it into two lists loses
// that pairing. The split is nevertheless the right answer to the question this
// type is for, which is the sudo one: "+name" in sudoUser reads only the user
// slots, and in sudoHost only the host slots. Anyone who needs the pairing
// should read the triples instead.
type NetgroupExpansion struct {
	// Root is the netgroup that was expanded.
	Root string `json:"root"`

	Users []NetgroupMember `json:"users"`
	Hosts []NetgroupMember `json:"hosts"`

	// OpenUser and OpenHost name the netgroups that contribute a triple with
	// that slot left open — which in NIS means "any", not "none".
	//
	// This is the distinction a list of names cannot carry: a netgroup with no
	// host triples and a netgroup whose host slots are all open both yield an
	// empty Hosts, and they mean opposite things. Reading Hosts without
	// consulting OpenHost is how a rule that matches every machine gets reported
	// as matching none.
	//
	// A "-" slot is neither: it matches nothing, so it contributes no name and
	// opens nothing.
	OpenUser []string `json:"openUser,omitempty"`
	OpenHost []string `json:"openHost,omitempty"`

	// Cycles records nesting edges that pointed back into the walk, as
	// "child → parent". Not an error: glibc's innetgr() bounds a loop rather than
	// failing on it, so the hosts apply such a configuration without complaint
	// and calling it broken here would contradict them. It is still worth
	// surfacing — nothing legitimate needs a cycle.
	Cycles []string `json:"cycles,omitempty"`

	// Dangling names memberNisNetgroup values with no netgroup behind them.
	// Shown rather than skipped, like a group membership whose uid resolves to
	// no user: a reference into nothing is something the operator has to see.
	Dangling []string `json:"dangling,omitempty"`
}

// AnyUser reports that this netgroup matches every user, because at least one
// contributing triple leaves the user slot open.
func (e *NetgroupExpansion) AnyUser() bool { return len(e.OpenUser) > 0 }

// AnyHost reports that this netgroup matches every host.
func (e *NetgroupExpansion) AnyHost() bool { return len(e.OpenHost) > 0 }

// netgroupResolver expands netgroups without going back to the directory for
// each one.
//
// The whole collection is read once and walked in memory, for two reasons: a
// recursive expansion would otherwise cost one search per node, and
// nisNetgroupTriple cannot be filtered server-side at all (see
// SearchNetgroupsPage), so the reverse lookups have to read everything anyway.
//
// One instance per request, like lockoutResolver: a listing of 500 users costs
// one read rather than 500, and a netgroup edited between two requests is picked
// up by the next one — nothing stale outlives the response it was read for.
type netgroupResolver struct {
	client *Client
	byName map[string]*Netgroup
	// parents maps a netgroup name to the netgroups that nest it, which is the
	// edge direction the reverse lookup walks.
	parents map[string][]string
	loaded  bool
}

func (c *Client) newNetgroupResolver() *netgroupResolver {
	return &netgroupResolver{client: c}
}

func (r *netgroupResolver) load() error {
	if r.loaded {
		return nil
	}
	all, err := r.client.ListNetgroups()
	if err != nil {
		return err
	}
	r.index(all)
	return nil
}

// index builds the lookup and the reverse nesting edges. Split out from load so
// the graph walk can be exercised on a graph built in memory, which is where the
// cases worth testing live — a directory cannot easily be made to hold a cycle
// on demand.
func (r *netgroupResolver) index(all []Netgroup) {
	r.byName = make(map[string]*Netgroup, len(all))
	r.parents = make(map[string][]string)
	for i := range all {
		netgroup := &all[i]
		r.byName[netgroup.CN] = netgroup
	}
	for i := range all {
		netgroup := &all[i]
		for _, child := range netgroup.Members {
			r.parents[child] = append(r.parents[child], netgroup.CN)
		}
	}
	r.loaded = true
}

// expand walks the nesting graph breadth-first from cn.
//
// Breadth-first rather than depth-first so that Via names the *nearest*
// netgroup that contributes a name: reached through two paths, a member is
// attributed to the shorter one, which is the one an operator would edit.
//
// Termination comes from the visited set, not from a depth limit. The graph is
// finite — it was loaded in full — so every node is entered at most once, and an
// arbitrary depth cap could only truncate a legitimately deep graph and report
// the result as though it were complete.
func (r *netgroupResolver) expand(cn string) (*NetgroupExpansion, error) {
	if err := r.load(); err != nil {
		return nil, err
	}
	if _, ok := r.byName[cn]; !ok {
		return nil, fmt.Errorf("netgroup %q not found", cn)
	}

	expansion := &NetgroupExpansion{Root: cn}
	users := make(map[string]NetgroupMember)
	hosts := make(map[string]NetgroupMember)
	openUser := make(map[string]bool)
	openHost := make(map[string]bool)
	danglingSeen := make(map[string]bool)

	visited := map[string]bool{cn: true}
	queue := []string{cn}

	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		netgroup := r.byName[name]

		// Attribution: the root names its own members directly.
		via := name
		if name == cn {
			via = ""
		}

		for _, triple := range netgroup.Triples {
			collectSlot(triple.User, name, via, users, openUser)
			collectSlot(triple.Host, name, via, hosts, openHost)
		}

		for _, child := range netgroup.Members {
			if _, ok := r.byName[child]; !ok {
				if !danglingSeen[child] {
					danglingSeen[child] = true
					expansion.Dangling = append(expansion.Dangling, child)
				}
				continue
			}
			if visited[child] {
				// Re-entering a netgroup already in the walk. Recorded as the edge
				// that closed the loop, which is what an operator has to go and
				// break.
				expansion.Cycles = append(expansion.Cycles, name+" → "+child)
				continue
			}
			visited[child] = true
			queue = append(queue, child)
		}
	}

	expansion.Users = sortedMembers(users)
	expansion.Hosts = sortedMembers(hosts)
	expansion.OpenUser = sortedKeys(openUser)
	expansion.OpenHost = sortedKeys(openHost)
	sort.Strings(expansion.Cycles)
	sort.Strings(expansion.Dangling)
	return expansion, nil
}

// collectSlot records what one slot of one triple contributes.
//
// The three cases are not interchangeable, and conflating the first two is the
// mistake this whole type exists to prevent:
//
//	""   the slot is open — matches every name, so it opens rather than adds
//	"-"  the NIS no-match marker — contributes nothing and opens nothing
//	name a real name
func collectSlot(value, owner, via string, into map[string]NetgroupMember, open map[string]bool) {
	switch value {
	case "":
		open[owner] = true
	case NetgroupWildcard:
		// Deliberately nothing.
	default:
		// First writer wins, and the walk is breadth-first, so this keeps the
		// nearest attribution.
		if _, seen := into[value]; !seen {
			into[value] = NetgroupMember{Name: value, Via: via}
		}
	}
}

func sortedMembers(from map[string]NetgroupMember) []NetgroupMember {
	members := make([]NetgroupMember, 0, len(from))
	for _, member := range from {
		members = append(members, member)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Name < members[j].Name })
	return members
}

func sortedKeys(from map[string]bool) []string {
	keys := make([]string, 0, len(from))
	for key := range from {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// NetgroupMembership is one netgroup that reaches a user, and how it does.
type NetgroupMembership struct {
	CN string `json:"cn"`
	// Direct is false when the user is reached only through a nesting parent.
	// The distinction is not cosmetic: "+parent" in a sudo rule grants to
	// everyone the parent absorbs, so an indirect membership still confers the
	// privilege — but the parent is not the entry to edit to take it away.
	Direct bool `json:"direct"`
}

// membershipsForUser returns every netgroup that reaches this user, marking
// those whose own triples name them.
//
// It walks the nesting edges upwards from the netgroups that name the user,
// rather than expanding every netgroup and testing each result: the edge walk is
// linear in the graph, where the other is quadratic in the number of netgroups.
//
// Both answers come out of one pass because they come out of one read. Asking
// the client for them separately meant loading the whole collection twice to
// compute the second from a subset of the first.
func (r *netgroupResolver) membershipsForUser(uid string) ([]NetgroupMembership, error) {
	if uid == "" {
		return nil, nil
	}
	if err := r.load(); err != nil {
		return nil, err
	}

	found := make(map[string]bool)
	var queue []string
	for name, netgroup := range r.byName {
		for _, triple := range netgroup.Triples {
			if triple.NamesUser(uid) {
				// Direct: this netgroup's own triples name the user.
				found[name] = true
				queue = append(queue, name)
				break
			}
		}
	}

	// Anything nesting a netgroup that contains the user contains them too. The
	// `found` map doubles as the visited set, so a cycle terminates here as well.
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		for _, parent := range r.parents[name] {
			if _, seen := found[parent]; seen {
				continue
			}
			found[parent] = false
			queue = append(queue, parent)
		}
	}

	names := sortedKeys(found)
	memberships := make([]NetgroupMembership, 0, len(names))
	for _, name := range names {
		memberships = append(memberships, NetgroupMembership{CN: name, Direct: found[name]})
	}
	return memberships, nil
}

// containingUser returns the netgroups that effectively contain this user, by
// name, including those that reach them only through nesting.
func (r *netgroupResolver) containingUser(uid string) ([]string, error) {
	memberships, err := r.membershipsForUser(uid)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(memberships))
	for _, membership := range memberships {
		names = append(names, membership.CN)
	}
	return names, nil
}

// ExpandNetgroup resolves one netgroup, following nesting.
//
// For a single lookup. Anything expanding several netgroups — or expanding one
// per row of a listing — should take a resolver instead, so the directory is
// read once rather than once per call.
func (c *Client) ExpandNetgroup(cn string) (*NetgroupExpansion, error) {
	return c.newNetgroupResolver().expand(cn)
}

// NetgroupsContainingUser returns the netgroups that reach this user, directly
// or through nesting, by name.
//
// The counterpart of NetgroupsForUser, which stops at direct membership. This is
// the one sudo needs: "+ops-oncall" grants to everyone ops-oncall absorbs.
func (c *Client) NetgroupsContainingUser(uid string) ([]string, error) {
	return c.newNetgroupResolver().containingUser(uid)
}

// NetgroupMembershipsForUser is NetgroupsContainingUser with direct membership
// distinguished, for the caller that needs both. One resolver, so one read.
func (c *Client) NetgroupMembershipsForUser(uid string) ([]NetgroupMembership, error) {
	return c.newNetgroupResolver().membershipsForUser(uid)
}
