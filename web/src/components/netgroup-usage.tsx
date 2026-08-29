import { useQueries, useQuery } from '@tanstack/react-query'
import { AlertTriangle, Network } from 'lucide-react'
import { api, type Netgroup, type NetgroupsResponse } from '@/lib/api'
import { Callout } from '@/components/ui/callout'
import { netgroupKind, type NetgroupKind } from '@/lib/netgroup'

/** The netgroup names a sudo field refers to, without their "+" prefix. */
function namedNetgroups(values: string[]): string[] {
  return values.filter((v) => v.startsWith('+') && v.length > 1).map((v) => v.slice(1))
}

/**
 * What the directory says about one "+name" on a rule.
 *
 * "absent" is a claim, not a default: it is only reached when the whole result
 * set for that name was seen. Anything else — still loading, or a result the
 * server had to cut short — is "unknown", because reporting a netgroup as
 * missing when it merely sat outside the page would condemn a rule that works.
 */
type Resolution =
  | { status: 'found'; netgroup: Netgroup }
  | { status: 'absent' }
  | { status: 'unknown' }

/**
 * Wide enough that an exact name is never crowded out in practice, since the
 * search also matches descriptions, triples and nested names — and small enough
 * that a rule naming a few netgroups stays cheap. When it is not enough the
 * answer degrades to "unknown" rather than to a wrong one.
 */
const NAME_LOOKUP_LIMIT = 50

function resolutionOf(name: string, page: NetgroupsResponse | undefined): Resolution {
  if (!page) return { status: 'unknown' }

  const exact = page.data.find((n) => n.cn === name)
  if (exact) return { status: 'found', netgroup: exact }

  // Nothing found only means "no such netgroup" if nothing was left out.
  // `truncated` reports the server-side size limit; comparing against `total`
  // catches the page being narrower than the match set.
  const complete = page.truncated !== true && page.data.length >= page.total
  return complete ? { status: 'absent' } : { status: 'unknown' }
}

/**
 * Resolves every netgroup a sudo rule names, on either side.
 *
 * One search per name rather than one page of the whole collection: a name is
 * looked up, not hunted for in whatever the first page happened to hold. The
 * page-based version silently reported any netgroup past its limit as
 * non-existent, which is the worst answer available here — it condemns a rule
 * that works, on a screen whose whole job is to say when one does not.
 */
function useNetgroupResolutions(sudoUser: string[], sudoHost: string[]) {
  const names = [...new Set([...namedNetgroups(sudoUser), ...namedNetgroups(sudoHost)])]

  const { data: config } = useQuery({
    queryKey: ['admin', 'config'],
    queryFn: ({ signal }) => api.admin.getConfig(signal),
  })
  const supported = config?.capabilities?.netgroupsSupported ?? false

  const results = useQueries({
    queries: names.map((name) => ({
      queryKey: ['netgroups', 'by-name', name],
      queryFn: ({ signal }: { signal: AbortSignal }) =>
        api.netgroups.list({ search: name, limit: NAME_LOOKUP_LIMIT }, signal),
      enabled: supported,
    })),
  })

  const byName = new Map<string, Resolution>()
  names.forEach((name, i) => byName.set(name, resolutionOf(name, results[i]?.data)))

  return {
    supported,
    names,
    resolve: (name: string): Resolution => byName.get(name) ?? { status: 'unknown' },
    settled: results.every((r) => !r.isPending),
  }
}

/**
 * Warns when a sudo rule names a netgroup on the side that will ignore it.
 *
 * The picker already checks this when a netgroup is chosen there, but both
 * fields stay free text — "+ops" can be typed, pasted, or have arrived from
 * another tool. Deriving the check from the stored values instead of from the
 * picker means it holds however the value got there, which is the only version
 * worth having: sudo reads only the user slots of a netgroup named in sudoUser
 * and only the host slots of one named in sudoHost, and a mismatch matches
 * nobody without a word from sudo or the directory.
 */
export function NetgroupUsageWarnings({
  sudoUser,
  sudoHost,
}: {
  sudoUser: string[]
  sudoHost: string[]
}) {
  const { supported, names, resolve, settled } = useNetgroupResolutions(sudoUser, sudoHost)
  if (!supported || names.length === 0 || !settled) return null

  const problems: { name: string; kind: NetgroupKind | null; side: 'Users' | 'Hosts' }[] = []

  const check = (values: string[], side: 'Users' | 'Hosts', wanted: NetgroupKind) => {
    for (const name of namedNetgroups(values)) {
      const resolution = resolve(name)
      // An unresolved name is not reported either way: it is the one state
      // that carries no claim.
      if (resolution.status === 'unknown') continue
      if (resolution.status === 'absent') {
        problems.push({ name, kind: null, side })
        continue
      }
      const kind = netgroupKind(resolution.netgroup)
      // "empty" contributes through nesting only, so its own triples say
      // nothing about which side it belongs on.
      if (kind !== wanted && kind !== 'empty') {
        problems.push({ name, kind, side })
      }
    }
  }
  check(sudoUser, 'Users', 'users')
  check(sudoHost, 'Hosts', 'hosts')

  if (problems.length === 0) return null

  return (
    <Callout variant="warning">
      <p className="flex items-center gap-1.5 font-medium">
        <AlertTriangle className="h-4 w-4 shrink-0" aria-hidden="true" />
        {problems.length === 1 ? 'A netgroup on this rule' : 'Netgroups on this rule'} will be
        ignored
      </p>
      <ul className="mt-1 space-y-1">
        {problems.map((problem) => (
          <li key={`${problem.side}:${problem.name}`} className="flex items-start gap-1.5">
            <Network className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" />
            <span>
              <span className="font-mono">+{problem.name}</span> in {problem.side}:{' '}
              {problem.kind === null
                ? 'no netgroup of that name exists, so it matches nobody.'
                : `it names ${problem.kind === 'users' ? 'users' : problem.kind === 'hosts' ? 'hosts' : 'both users and hosts'}, and ${problem.side} reads the ${
                    problem.side === 'Users' ? 'user' : 'host'
                  } slots — ${
                    problem.kind === 'mixed'
                      ? 'only that half applies.'
                      : 'which are empty, so it matches nobody.'
                  }`}
            </span>
          </li>
        ))}
      </ul>
    </Callout>
  )
}

/**
 * Who a rule's netgroups actually reach.
 *
 * On a sudo rule a netgroup is only a string, so answering "who does this grant
 * to" meant remembering the name, leaving the page and opening the netgroup.
 * The expansion already existed for its own page; this brings it to where the
 * grant is being read.
 *
 * Names only, and only the side that applies: the full picture — provenance,
 * cycles, dangling nested names — stays on the netgroup's own page, one click
 * away.
 */
export function NetgroupContents({
  sudoUser,
  sudoHost,
}: {
  sudoUser: string[]
  sudoHost: string[]
}) {
  const { supported, names, resolve } = useNetgroupResolutions(sudoUser, sudoHost)
  if (!supported || names.length === 0) return null

  return (
    <div className="space-y-2">
      {namedNetgroups(sudoUser).map((name) => (
        <NetgroupSummary key={`u:${name}`} name={name} side="users" resolution={resolve(name)} />
      ))}
      {namedNetgroups(sudoHost).map((name) => (
        <NetgroupSummary key={`h:${name}`} name={name} side="hosts" resolution={resolve(name)} />
      ))}
    </div>
  )
}

function NetgroupSummary({
  name,
  side,
  resolution,
}: {
  name: string
  side: 'users' | 'hosts'
  resolution: Resolution
}) {
  const netgroup = resolution.status === 'found' ? resolution.netgroup : undefined

  const { data: expansion } = useQuery({
    queryKey: ['netgroup', netgroup?.dn, 'expand'],
    queryFn: ({ signal }) => api.netgroups.expand(netgroup!.dn, signal),
    enabled: !!netgroup,
  })

  // Absence is stated by the warning above, in the terms that matter there.
  if (resolution.status === 'absent') return null

  const members = side === 'users' ? expansion?.users : expansion?.hosts
  const open = side === 'users' ? expansion?.openUser : expansion?.openHost
  const matchesAll = (open?.length ?? 0) > 0

  return (
    <div className="rounded-md border px-3 py-2 text-sm">
      <p className="flex items-center gap-1.5 font-medium">
        <Network className="h-3.5 w-3.5 text-muted-foreground" aria-hidden="true" />
        +{name}
        <span className="text-xs font-normal text-muted-foreground">
          {side === 'users' ? 'grants to' : 'restricts to'}
        </span>
      </p>
      {resolution.status === 'unknown' ? (
        // Said rather than left blank: this is the state where the app does not
        // know, and a silent card would read as "nothing to report".
        <p className="text-xs text-muted-foreground">
          Could not be looked up — this directory holds more netgroups than a single search
          returns. Open the netgroup itself to see what it contains.
        </p>
      ) : !expansion ? (
        <p className="text-xs text-muted-foreground">Resolving…</p>
      ) : matchesAll ? (
        // The case a list of names cannot state: an open slot matches
        // everything, so naming members here would understate the rule.
        <p className="text-xs text-warning">
          every {side === 'users' ? 'user' : 'host'} — {open?.join(', ')} leave the slot open
        </p>
      ) : members && members.length > 0 ? (
        <p className="text-xs text-muted-foreground">{members.map((m) => m.name).join(', ')}</p>
      ) : (
        <p className="text-xs text-muted-foreground">
          nothing — this netgroup names no {side === 'users' ? 'user' : 'host'}
        </p>
      )}
    </div>
  )
}
