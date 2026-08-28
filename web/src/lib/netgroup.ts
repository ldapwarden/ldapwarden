import type { Netgroup, NetgroupTriple, NetgroupExpansion } from './api'

/**
 * The NIS marker for a slot that must match nothing, as opposed to an empty slot
 * which matches everything. The two are never interchangeable.
 */
export const NETGROUP_NO_MATCH = '-'

export type NetgroupKind = 'users' | 'hosts' | 'mixed' | 'empty'

/**
 * What a netgroup's own triples make it usable for.
 *
 * Derived, not declared: the schema has no notion of a user netgroup or a host
 * netgroup — both are nisNetgroup, and the difference is only which slot people
 * fill in. It matters because sudo reads "+name" from sudoUser and from
 * sudoHost, and a netgroup used on the wrong side matches nothing at all,
 * silently.
 *
 * This mirrors Netgroup.Kind on the server; it is recomputed here so a list row
 * does not need a second request.
 */
export function netgroupKind(netgroup: Netgroup): NetgroupKind {
  let users = false
  let hosts = false
  for (const triple of netgroup.triples) {
    if (triple.user && triple.user !== NETGROUP_NO_MATCH) users = true
    if (triple.host && triple.host !== NETGROUP_NO_MATCH) hosts = true
  }
  if (users && hosts) return 'mixed'
  if (users) return 'users'
  if (hosts) return 'hosts'
  return 'empty'
}

export function netgroupKindLabel(kind: NetgroupKind): string {
  switch (kind) {
    case 'users':
      return 'Names users, so it belongs in a sudo rule’s Users field as “+name”.'
    case 'hosts':
      return 'Names hosts, so it belongs in a sudo rule’s Hosts field as “+name”.'
    case 'mixed':
      return 'Names both users and hosts. Used from a sudo rule, only the matching half applies — the rest is ignored without warning.'
    case 'empty':
      return 'Names nothing of its own; whatever it holds comes from the netgroups it nests.'
  }
}

/**
 * How one triple reads in prose.
 *
 * An empty slot is the trap: it means "any", never "none". Rendering it as a
 * blank cell would invite exactly the wrong reading, so it is spelled out.
 */
export function describeTriple(triple: NetgroupTriple): string {
  const who = slotLabel(triple.user, 'any user', 'no user')
  const where = slotLabel(triple.host, 'any host', 'no host')
  return `${who} on ${where}`
}

function slotLabel(value: string, open: string, none: string): string {
  if (value === '') return open
  if (value === NETGROUP_NO_MATCH) return none
  return value
}

/** The stored spelling, for anyone comparing against the directory. */
export function formatTriple(triple: NetgroupTriple): string {
  return `(${triple.host},${triple.user},${triple.domain})`
}

/**
 * Whether an expansion matches every host, which an empty host list cannot say
 * on its own — the netgroup with no host triples and the one whose host slots
 * are all open both produce an empty list and mean opposite things.
 */
export function matchesAnyHost(expansion: NetgroupExpansion): boolean {
  return (expansion.openHost?.length ?? 0) > 0
}

export function matchesAnyUser(expansion: NetgroupExpansion): boolean {
  return (expansion.openUser?.length ?? 0) > 0
}
