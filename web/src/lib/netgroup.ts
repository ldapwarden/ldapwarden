import type { Netgroup, NetgroupTriple, NetgroupExpansion, NetgroupsResponse } from './api'

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

/** The half of a netgroup a sudo field reads. */
export type NetgroupSide = 'users' | 'hosts'

/**
 * How much of a netgroup applies when it is named on a given side.
 *
 * `sudoUser` reads only the user slots of a "+name" and `sudoHost` only the host
 * slots, so the same reference can be the whole grant, half of it, or nothing at
 * all — and sudo reports none of those cases.
 *
 * "partial" is not "ignored": a netgroup naming both users and hosts still works
 * from either side, with the other half silently dropped. Collapsing the two
 * would tell an operator their rule matches nobody when it matches half of what
 * they wrote. "empty" is "ok" because a netgroup with no triples of its own says
 * nothing about which side it belongs on — whatever it holds comes from nesting.
 */
export type NetgroupSideFit = 'ok' | 'ignored' | 'partial'

export function netgroupSideFit(kind: NetgroupKind, side: NetgroupSide): NetgroupSideFit {
  if (kind === 'empty') return 'ok'
  if (kind === 'mixed') return 'partial'
  return kind === side ? 'ok' : 'ignored'
}

/**
 * What a search could establish about a "+name" reference.
 *
 * "absent" is a claim and has to be earned: it means the whole result set was
 * seen and the name was not in it. A page that the server truncated, or one
 * narrower than the match set, proves nothing — reporting a netgroup as missing
 * because it sat outside the page would condemn a rule that works.
 */
export type NetgroupResolution =
  | { status: 'found'; netgroup: Netgroup }
  | { status: 'absent' }
  | { status: 'unknown' }

export function resolveNetgroupByName(
  name: string,
  page: NetgroupsResponse | undefined,
): NetgroupResolution {
  if (!page) return { status: 'unknown' }

  const exact = page.data.find((n) => n.cn === name)
  if (exact) return { status: 'found', netgroup: exact }

  const complete = page.truncated !== true && page.data.length >= page.total
  return complete ? { status: 'absent' } : { status: 'unknown' }
}

/** The netgroup names a sudo field refers to, without their "+" prefix. */
export function namedNetgroups(values: string[]): string[] {
  return values.filter((v) => v.startsWith('+') && v.length > 1).map((v) => v.slice(1))
}

/**
 * A triple in its stored spelling, for validating text before it is sent.
 *
 * Shape only — parentheses and exactly two commas. The slots are free-form
 * names, and the server parses every value with the canonical parser anyway;
 * this exists to catch a CSV column mapped to the wrong thing before a whole
 * file is submitted.
 */
export const NETGROUP_TRIPLE_TEXT = /^\([^,()]*,[^,()]*,[^,()]*\)$/
