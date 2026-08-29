import { describe, expect, it } from 'vitest'
import type { Netgroup, NetgroupExpansion, NetgroupsResponse, NetgroupTriple } from './api'
import {
  DEFAULT_MODULES,
  NETGROUP_NO_MATCH,
  NETGROUP_TRIPLE_TEXT,
  describeTriple,
  formatTriple,
  matchesAnyHost,
  matchesAnyUser,
  namedNetgroups,
  netgroupKind,
  netgroupSideFit,
  netgroupsEnabled,
  resolveNetgroupByName,
} from './netgroup'

function triple(parts: Partial<NetgroupTriple>): NetgroupTriple {
  return { host: '', user: '', domain: '', ...parts }
}

function netgroup(cn: string, triples: NetgroupTriple[]): Netgroup {
  return { dn: `cn=${cn},ou=netgroups,dc=example,dc=org`, cn, triples }
}

function expansion(over: Partial<NetgroupExpansion>): NetgroupExpansion {
  return { root: 'ops', users: [], hosts: [], ...over }
}

function page(over: Partial<NetgroupsResponse>): NetgroupsResponse {
  const data = over.data ?? []
  return { data, total: over.total ?? data.length, truncated: over.truncated }
}

describe('netgroupKind', () => {
  it('derives the kind from which slots are filled', () => {
    expect(netgroupKind(netgroup('a', [triple({ user: 'jdoe' })]))).toBe('users')
    expect(netgroupKind(netgroup('a', [triple({ host: 'web01' })]))).toBe('hosts')
    expect(netgroupKind(netgroup('a', [triple({ host: 'web01', user: 'jdoe' })]))).toBe('mixed')
    expect(netgroupKind(netgroup('a', []))).toBe('empty')
  })

  // The trap this whole module exists for: "-" is a filled slot as far as the
  // syntax goes, and means the opposite of a name.
  it('does not count a "-" slot as naming anything', () => {
    const wildcardHost = netgroup('a', [triple({ host: NETGROUP_NO_MATCH, user: 'jdoe' })])
    expect(netgroupKind(wildcardHost)).toBe('users')

    const wildcardBoth = netgroup('a', [
      triple({ host: NETGROUP_NO_MATCH, user: NETGROUP_NO_MATCH }),
    ])
    expect(netgroupKind(wildcardBoth)).toBe('empty')
  })
})

describe('describeTriple', () => {
  // An empty slot means "any" and "-" means "none". Rendering either as a blank
  // invites the exact opposite reading.
  it.each([
    [{ user: 'jdoe' }, 'jdoe on any host'],
    [{ host: NETGROUP_NO_MATCH, user: 'jdoe' }, 'jdoe on no host'],
    [{ host: 'web01' }, 'any user on web01'],
    [{}, 'any user on any host'],
    [{ host: NETGROUP_NO_MATCH, user: NETGROUP_NO_MATCH }, 'no user on no host'],
  ])('%o reads as "%s"', (parts, expected) => {
    expect(describeTriple(triple(parts))).toBe(expected)
  })

  it('distinguishes the two one-character-apart spellings', () => {
    expect(describeTriple(triple({ user: 'jdoe' }))).not.toBe(
      describeTriple(triple({ host: NETGROUP_NO_MATCH, user: 'jdoe' })),
    )
  })
})

describe('formatTriple', () => {
  it('renders the stored spelling, keeping empty slots empty', () => {
    expect(formatTriple(triple({ user: 'jdoe' }))).toBe('(,jdoe,)')
    expect(formatTriple(triple({ host: NETGROUP_NO_MATCH, user: 'jdoe' }))).toBe('(-,jdoe,)')
    expect(formatTriple(triple({ host: 'web01', user: 'bob', domain: 'example.org' }))).toBe(
      '(web01,bob,example.org)',
    )
  })

  it('round-trips through the shape check the CSV import applies', () => {
    for (const parts of [{ user: 'jdoe' }, { host: NETGROUP_NO_MATCH }, {}]) {
      expect(NETGROUP_TRIPLE_TEXT.test(formatTriple(triple(parts)))).toBe(true)
    }
  })
})

describe('NETGROUP_TRIPLE_TEXT', () => {
  it.each(['(,jdoe,)', '(-,jdoe,)', '(web01,,)', '(,,)', '(web01,bob,example.org)'])(
    'accepts %s',
    (value) => expect(NETGROUP_TRIPLE_TEXT.test(value)).toBe(true),
  )

  it.each([
    ['(web01,jdoe)', 'two fields instead of three'],
    ['web01,jdoe,', 'no parentheses'],
    ['(web01,jdoe,example.org', 'unclosed'],
    ['(a,b,c,d)', 'four fields'],
    ['(a,(b),c)', 'nested parentheses'],
    ['', 'empty cell'],
  ])('rejects %s (%s)', (value) => expect(NETGROUP_TRIPLE_TEXT.test(value)).toBe(false))
})

describe('netgroupSideFit', () => {
  it('matches when the kind is the side', () => {
    expect(netgroupSideFit('users', 'users')).toBe('ok')
    expect(netgroupSideFit('hosts', 'hosts')).toBe('ok')
  })

  // Naming a netgroup of users in sudoHost matches nothing at all, and neither
  // sudo nor the directory says a word about it.
  it('reports the wrong side as ignored', () => {
    expect(netgroupSideFit('users', 'hosts')).toBe('ignored')
    expect(netgroupSideFit('hosts', 'users')).toBe('ignored')
  })

  // "partial" is not "ignored": half the netgroup still applies, and telling an
  // operator it matches nobody would be false.
  it('reports a mixed netgroup as partial on either side', () => {
    expect(netgroupSideFit('mixed', 'users')).toBe('partial')
    expect(netgroupSideFit('mixed', 'hosts')).toBe('partial')
  })

  // Its own triples say nothing about which side it belongs on; whatever it
  // holds comes from nesting, so no mismatch can be claimed.
  it('never blames a netgroup with no triples of its own', () => {
    expect(netgroupSideFit('empty', 'users')).toBe('ok')
    expect(netgroupSideFit('empty', 'hosts')).toBe('ok')
  })
})

describe('namedNetgroups', () => {
  it('takes the "+" values and strips the prefix', () => {
    expect(namedNetgroups(['jdoe', '+ops', '%developers', '+web-servers'])).toEqual([
      'ops',
      'web-servers',
    ])
  })

  it('ignores a bare "+", which names nothing', () => {
    expect(namedNetgroups(['+'])).toEqual([])
  })
})

describe('resolveNetgroupByName', () => {
  const ops = netgroup('ops', [triple({ user: 'jdoe' })])

  it('finds an exact match, not a prefix', () => {
    const result = resolveNetgroupByName('ops', page({ data: [netgroup('ops-oncall', []), ops] }))
    expect(result).toEqual({ status: 'found', netgroup: ops })
  })

  it('is unknown until the page arrives', () => {
    expect(resolveNetgroupByName('ops', undefined)).toEqual({ status: 'unknown' })
  })

  it('claims absence only from a complete result set', () => {
    expect(resolveNetgroupByName('ops', page({ data: [], total: 0 }))).toEqual({ status: 'absent' })
  })

  // The bug this function was extracted from: concluding "no such netgroup"
  // from a page that simply did not reach it condemns a rule that works.
  it('does not claim absence when the page is narrower than the match set', () => {
    const partial = page({ data: [netgroup('a-ops', [])], total: 3 })
    expect(resolveNetgroupByName('ops', partial)).toEqual({ status: 'unknown' })
  })

  it('does not claim absence when the server truncated the search', () => {
    const cut = page({ data: [], total: 0, truncated: true })
    expect(resolveNetgroupByName('ops', cut)).toEqual({ status: 'unknown' })
  })
})

describe('matchesAnyHost / matchesAnyUser', () => {
  // An empty list and an open slot both produce no names and mean opposite
  // things, which is why the expansion carries openHost/openUser at all.
  it('reads an open slot, which an empty list cannot express', () => {
    expect(matchesAnyHost(expansion({ hosts: [], openHost: ['ops'] }))).toBe(true)
    expect(matchesAnyHost(expansion({ hosts: [] }))).toBe(false)
    expect(matchesAnyUser(expansion({ users: [], openUser: ['ops'] }))).toBe(true)
    expect(matchesAnyUser(expansion({ users: [] }))).toBe(false)
  })
})

describe('netgroupsEnabled', () => {
  it('needs both the module and the directory', () => {
    expect(netgroupsEnabled(['users', 'netgroups'], true)).toBe(true)
    expect(netgroupsEnabled(['users', 'netgroups'], false)).toBe(false)
    expect(netgroupsEnabled(['users'], true)).toBe(false)
  })
})

describe('DEFAULT_MODULES', () => {
  // The fallback used when the config endpoint is unreadable. It drifted from
  // the backend default once already, and the symptom was invisible: a
  // non-admin session simply lost a tab.
  it('matches the backend default in internal/config/config.go', () => {
    expect(DEFAULT_MODULES).toEqual(['users', 'groups', 'sudo', 'policies', 'netgroups'])
  })
})
