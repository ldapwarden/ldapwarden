import { AlertTriangle, Globe, Server, User } from 'lucide-react'
import type { NetgroupExpansion } from '@/lib/api'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Callout } from '@/components/ui/callout'
import { Skeleton } from '@/components/ui/skeleton'
import { matchesAnyHost, matchesAnyUser } from '@/lib/netgroup'

/**
 * What the netgroup actually contributes, nesting followed.
 *
 * The panel exists because the entry alone does not answer the question an
 * operator has — "who does this grant to, in the end" — and because two facts
 * are invisible without it: names inherited from a nested netgroup, and slots
 * left open. An open slot matches *every* name, so a netgroup can contribute
 * nothing to the host list and still apply to every machine; showing an empty
 * list there would be the exact inverse of the truth.
 */
export function NetgroupExpansionPanel({ expansion }: { expansion: NetgroupExpansion | undefined }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Effective contents</CardTitle>
        <CardDescription>Nested netgroups followed, as sudo would see them</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {!expansion ? (
          <div className="space-y-2">
            <span className="sr-only" role="status">
              Resolving…
            </span>
            <Skeleton className="h-5 w-2/3" />
            <Skeleton className="h-5 w-1/2" />
          </div>
        ) : (
          <>
            <Side
              icon={User}
              title="Users"
              members={expansion.users}
              open={matchesAnyUser(expansion)}
              openVia={expansion.openUser ?? []}
              anyLabel="Matches every user"
              emptyLabel="No user"
              usedFrom="the Users field of a sudo rule"
            />
            <Side
              icon={Server}
              title="Hosts"
              members={expansion.hosts}
              open={matchesAnyHost(expansion)}
              openVia={expansion.openHost ?? []}
              anyLabel="Matches every host"
              emptyLabel="No host"
              usedFrom="the Hosts field of a sudo rule"
            />

            {expansion.cycles && expansion.cycles.length > 0 && (
              <Callout variant="warning">
                <p className="font-medium">This netgroup nests itself</p>
                <p className="mt-1">
                  Resolution stops rather than looping, and the hosts do the same, so nothing is
                  broken — but nothing legitimate needs a loop either.
                </p>
                <ul className="mt-2 space-y-0.5 font-mono text-xs">
                  {expansion.cycles.map((cycle) => (
                    <li key={cycle}>{cycle}</li>
                  ))}
                </ul>
              </Callout>
            )}

            {expansion.dangling && expansion.dangling.length > 0 && (
              <Callout variant="warning">
                <p className="font-medium">
                  {expansion.dangling.length} nested netgroup
                  {expansion.dangling.length === 1 ? '' : 's'} do not exist
                </p>
                <p className="mt-1">
                  The reference is kept, and contributes nothing. Most often the netgroup was
                  deleted and the name left behind.
                </p>
                <ul className="mt-2 space-y-0.5 font-mono text-xs">
                  {expansion.dangling.map((name) => (
                    <li key={name}>{name}</li>
                  ))}
                </ul>
              </Callout>
            )}
          </>
        )}
      </CardContent>
    </Card>
  )
}

function Side({
  icon: Icon,
  title,
  members,
  open,
  openVia,
  anyLabel,
  emptyLabel,
  usedFrom,
}: {
  icon: typeof User
  title: string
  members: NetgroupExpansion['users']
  open: boolean
  openVia: string[]
  anyLabel: string
  emptyLabel: string
  usedFrom: string
}) {
  return (
    <div className="space-y-1.5">
      <p className="flex items-center gap-1.5 text-sm font-medium">
        <Icon className="h-4 w-4 text-muted-foreground" aria-hidden="true" />
        {title}
        {!open && <span className="text-muted-foreground">({members.length})</span>}
      </p>

      {open ? (
        // The case a list cannot express. Stated before the names, because it
        // overrides them: whatever else is listed, this matches everything.
        <p className="flex items-start gap-1.5 rounded-md bg-warning/10 px-2 py-1.5 text-sm text-warning">
          <Globe className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <span>
            {anyLabel} — {openVia.join(', ')} leave{openVia.length === 1 ? 's' : ''} the slot open.
            Used from {usedFrom}, this places no restriction.
          </span>
        </p>
      ) : members.length === 0 ? (
        <p className="text-sm text-muted-foreground">{emptyLabel}</p>
      ) : null}

      {members.length > 0 && (
        <ul className="flex flex-wrap gap-1.5">
          {members.map((member) => (
            <li
              key={member.name}
              className="inline-flex items-center gap-1 rounded-md bg-muted px-2 py-0.5 text-sm"
              title={member.via ? `Inherited from ${member.via}` : 'Named by this netgroup'}
            >
              {member.name}
              {member.via && (
                <span className="text-xs text-muted-foreground">via {member.via}</span>
              )}
            </li>
          ))}
        </ul>
      )}

      {open && members.length > 0 && (
        <p className="flex items-start gap-1.5 text-xs text-muted-foreground">
          <AlertTriangle className="mt-0.5 h-3 w-3 shrink-0" aria-hidden="true" />
          The names above are redundant while the slot is open.
        </p>
      )}
    </div>
  )
}
