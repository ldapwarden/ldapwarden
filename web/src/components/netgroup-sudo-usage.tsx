import { Link } from '@tanstack/react-router'
import { ShieldCheck, AlertTriangle } from 'lucide-react'
import type { SudoRole } from '@/lib/api'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Callout } from '@/components/ui/callout'
import { Button } from '@/components/ui/button'
import { netgroupKind, netgroupSideFit, type NetgroupKind } from '@/lib/netgroup'
import { encodeDN } from '@/lib/utils'
import type { Netgroup } from '@/lib/api'

/**
 * The sudo rules that name this netgroup.
 *
 * A netgroup exists to be referenced, but the reference is a bare "+name" in
 * someone else's entry and nothing in the directory ties it back here. So the
 * question this panel answers — what does this netgroup actually grant — used to
 * be answerable only by searching the sudo rules by hand, or by starting a
 * delete and reading the warning.
 *
 * Which side each rule names it on is the part that decides the answer: sudo
 * reads only the user slots of a "+name" in sudoUser and only the host slots of
 * one in sudoHost, so the same reference means different things and can mean
 * nothing at all.
 */
export function NetgroupSudoUsage({
  netgroup,
  roles,
  loading,
  error,
  onRetry,
}: {
  netgroup: Netgroup
  roles: SudoRole[] | undefined
  loading: boolean
  /**
   * Why the search produced nothing, when that is the reason.
   *
   * This panel's empty state is a claim — "no sudo rule names this netgroup",
   * which an operator may act on. Without knowing the search failed, a failure
   * rendered as that claim: confidently telling someone a netgroup grants
   * nothing, when in fact nobody had looked.
   */
  error?: Error | null
  onRetry?: () => void
}) {
  const reference = '+' + netgroup.cn
  const kind = netgroupKind(netgroup)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldCheck className="h-4 w-4 text-muted-foreground" aria-hidden="true" />
          Used by sudo ({roles?.length ?? 0})
        </CardTitle>
        <CardDescription>
          Rules naming <span className="font-mono">{reference}</span>. The directory does not track
          this reference, so it is found by searching the sudo rules.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        {error ? (
          <Callout variant="error">
            <p className="font-medium">The sudo rules could not be searched.</p>
            <p className="mt-1">{error.message}</p>
            <p className="mt-1">
              This is not the same as “no rule names it” — nothing was established either way.
            </p>
            {onRetry && (
              <Button variant="outline" size="sm" className="mt-2" onClick={onRetry}>
                Try again
              </Button>
            )}
          </Callout>
        ) : loading ? (
          <p className="text-sm text-muted-foreground">Searching…</p>
        ) : !roles || roles.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No sudo rule names {reference}. This netgroup grants nothing on its own — it can still
            reach people through a netgroup that nests it.
          </p>
        ) : (
          roles.map((role) => (
            <RuleRow key={role.dn} role={role} reference={reference} kind={kind} />
          ))
        )}
      </CardContent>
    </Card>
  )
}

function RuleRow({
  role,
  reference,
  kind,
}: {
  role: SudoRole
  reference: string
  kind: NetgroupKind
}) {
  const inUsers = (role.sudoUser ?? []).includes(reference)
  const inHosts = (role.sudoHost ?? []).includes(reference)

  // Only when the rule names it on exactly one side: named on both, whichever
  // half is unused is used by the other reference.
  const ignored =
    inUsers !== inHosts && netgroupSideFit(kind, inUsers ? 'users' : 'hosts') === 'ignored'

  return (
    <div className="rounded-md border px-3 py-2 text-sm">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Link
          to="/sudo-roles/$dn"
          params={{ dn: encodeDN(role.dn) }}
          className="font-medium hover:underline"
        >
          {role.cn}
        </Link>
        <span className="text-xs text-muted-foreground">
          {inUsers && inHosts
            ? 'named in Users and Hosts'
            : inUsers
              ? 'grants to its users'
              : 'restricts to its hosts'}
        </span>
      </div>
      {role.description && (
        <p className="mt-0.5 text-xs text-muted-foreground">{role.description}</p>
      )}
      {ignored && (
        <Callout variant="warning" className="mt-2">
          <p className="flex items-start gap-1.5 text-xs">
            <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" />
            <span>
              This netgroup names {kind === 'users' ? 'users' : 'hosts'}, and the rule reads its{' '}
              {inUsers ? 'user' : 'host'} slots — which are empty, so the rule matches nobody
              through it.
            </span>
          </p>
        </Callout>
      )}
    </div>
  )
}
