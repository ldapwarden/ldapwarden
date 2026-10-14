import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { Fragment, useCallback, useRef, useState } from 'react'
import { ChevronDown, ChevronRight, DatabaseZap, Search } from 'lucide-react'
import { api, type DirectoryChange, type DirectoryChangeOrigin, type DirectoryChangesParams } from '@/lib/api'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Pagination } from '@/components/ui/pagination'
import { Select } from '@/components/ui/select'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { DatePicker } from '@/components/ui/date-picker'
import { Callout } from '@/components/ui/callout'
import { TableEmptyState } from '@/components/ui/empty-state'
import { TableSkeletonRows } from '@/components/ui/skeleton'
import { toneStyle, type Tone } from '@/lib/tones'
import { DEFAULT_PAGE_SIZE } from '@/lib/list-search'
import { useUrlSearchInput } from '@/lib/use-url-search-input'
import { useSearchHotkey } from '@/lib/use-hotkey'
import {
  CHANGE_TYPE_OPTIONS,
  ORIGIN_OPTIONS,
  type DirectoryChangesSearch,
  type OriginFilter,
} from '@/lib/directory-changes'

const ORIGIN_LABELS: Record<DirectoryChangeOrigin, string> = {
  external: 'Outside ldapwarden',
  ldapwarden: 'ldapwarden',
  policy: 'Password policy',
}

const ORIGIN_CLASSES: Record<DirectoryChangeOrigin, string> = {
  external: 'border-warning/40 bg-warning/10 text-warning',
  ldapwarden: 'border-success/40 bg-success/10 text-success',
  policy: 'border-border bg-muted text-muted-foreground',
}

const TYPE_TONES: Record<string, Tone> = { add: 'emerald', modify: 'blue', delete: 'rose', modrdn: 'violet' }

const OP_LABELS: Record<string, string> = { add: 'added', delete: 'removed', replace: 'set to', increment: 'incremented by' }

// The first RDN's value: "jdoe" for uid=jdoe,ou=People,… — what the table has
// room for. The full DN is in the title and the expanded row.
function shortName(dn: string): string {
  const first = dn.split(',')[0] ?? ''
  const eq = first.indexOf('=')
  return eq >= 0 ? first.slice(eq + 1) : first || '—'
}

function summarise(change: DirectoryChange): string {
  if (change.type === 'modrdn') return `renamed to ${change.newRdn ?? '?'}`
  if (change.type === 'delete') return 'entry deleted'
  const attributes = [...new Set(change.changes.map((c) => c.attribute))]
  if (attributes.length === 0) return '—'
  return attributes.length <= 3 ? attributes.join(', ') : `${attributes.length} attributes: ${attributes.join(', ')}`
}

export function DirectoryChangesView({
  search,
  onSearchChange,
}: {
  search: DirectoryChangesSearch
  onSearchChange: (patch: Partial<DirectoryChangesSearch>, replace?: boolean) => void
}) {
  const origin = search.origin ?? 'external'
  const type = search.type ?? ''
  const from = search.from ?? ''
  const to = search.to ?? ''
  const q = search.q ?? ''
  const currentPage = search.page ?? 1
  const pageSize = search.size ?? DEFAULT_PAGE_SIZE
  const hasFilters = Boolean(search.origin || type || from || to || q)

  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [searchInput, setSearchInput] = useUrlSearchInput(
    q,
    useCallback((value: string) => onSearchChange({ q: value || undefined, page: undefined }), [onSearchChange]),
  )
  const searchRef = useRef<HTMLInputElement>(null)
  useSearchHotkey(searchRef)

  const params: DirectoryChangesParams = {
    origin,
    type: type || undefined,
    search: q || undefined,
    from: from || undefined,
    to: to || undefined,
  }
  const { data, isLoading, isFetching, error } = useQuery({
    queryKey: ['directory-changes', { ...params, currentPage, pageSize }],
    queryFn: ({ signal }) =>
      api.directoryChanges.list({ ...params, limit: pageSize, offset: (currentPage - 1) * pageSize }, signal),
    placeholderData: keepPreviousData,
  })

  if (error) {
    return <Callout variant="error">Failed to load directory changes: {error.message}</Callout>
  }
  if (data?.status === 'not-configured') {
    return <AccessLogSetup bindDn={data.bindDn} />
  }
  if (data?.status === 'unreadable') {
    return <AccessLogUnreadable base={data.base ?? ''} bindDn={data.bindDn} />
  }

  const changes = data?.data ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const counts = data?.counts ?? {}

  const toggleExpanded = (id: string) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const clearFilters = () => {
    setSearchInput('')
    onSearchChange({ origin: undefined, type: undefined, q: undefined, from: undefined, to: undefined, page: undefined })
  }

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        Every write the directory accepted, from its own log — including the ones made with ldapmodify, scripts or
        other tools, which this app's audit log cannot know about.
        {!from && !to && ' Last 24 hours unless you pick dates.'}
      </p>

      <div className="flex flex-wrap items-end gap-3">
        <div className="space-y-1">
          <label htmlFor="changes-search" className="text-xs font-medium text-muted-foreground">
            Entry or actor
          </label>
          <div className="relative">
            <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input
              id="changes-search"
              ref={searchRef}
              className="w-56 pl-8"
              placeholder="DN fragment…"
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
            />
          </div>
        </div>
        <div className="space-y-1">
          <label htmlFor="changes-origin" className="text-xs font-medium text-muted-foreground">Made</label>
          <Select
            id="changes-origin"
            className="w-48"
            options={ORIGIN_OPTIONS}
            value={origin}
            onChange={(e) =>
              onSearchChange({
                origin: e.target.value === 'external' ? undefined : (e.target.value as OriginFilter),
                page: undefined,
              })
            }
          />
        </div>
        <div className="space-y-1">
          <label htmlFor="changes-type" className="text-xs font-medium text-muted-foreground">Operation</label>
          <Select
            id="changes-type"
            className="w-40"
            options={CHANGE_TYPE_OPTIONS}
            value={type}
            onChange={(e) => onSearchChange({ type: e.target.value || undefined, page: undefined })}
          />
        </div>
        <div className="space-y-1">
          <label htmlFor="changes-from" className="text-xs font-medium text-muted-foreground">From</label>
          <DatePicker
            id="changes-from"
            className="w-40"
            value={from}
            max={to || undefined}
            onChange={(value) => onSearchChange({ from: value || undefined, page: undefined })}
          />
        </div>
        <div className="space-y-1">
          <label htmlFor="changes-to" className="text-xs font-medium text-muted-foreground">To</label>
          <DatePicker
            id="changes-to"
            className="w-40"
            value={to}
            min={from || undefined}
            onChange={(value) => onSearchChange({ to: value || undefined, page: undefined })}
          />
        </div>
        {hasFilters && (
          <Button variant="ghost" size="sm" onClick={clearFilters}>
            Clear filters
          </Button>
        )}
      </div>

      {data && (
        <p className="text-sm text-muted-foreground">
          In this window: {counts.external ?? 0} outside ldapwarden, {counts.ldapwarden ?? 0} through ldapwarden,{' '}
          {counts.policy ?? 0} by the password policy.
        </p>
      )}
      {data?.truncated && (
        <Callout variant="warning">
          This window holds more writes than can be examined at once; only the most recent are listed. Pick a
          narrower date range to see the rest.
        </Callout>
      )}

      <div className={`border rounded-lg ${isFetching && !isLoading ? 'opacity-60 transition-opacity' : ''}`}>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-10" />
              <TableHead>Timestamp</TableHead>
              <TableHead>Actor</TableHead>
              <TableHead>Operation</TableHead>
              <TableHead>Entry</TableHead>
              <TableHead className="hidden md:table-cell">Made</TableHead>
              <TableHead className="hidden md:table-cell">Change</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableSkeletonRows rows={Math.min(pageSize, 8)} columns={7} hideBelowMd={[5, 6]} />
            ) : (
              <>
                {changes.map((change) => {
                  const isOpen = expanded.has(change.id)
                  return (
                    <Fragment key={change.id}>
                      <TableRow>
                        <TableCell className="w-10 align-top">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-6 w-6"
                            aria-expanded={isOpen}
                            aria-label={isOpen ? 'Hide details' : 'Show details'}
                            onClick={() => toggleExpanded(change.id)}
                          >
                            {isOpen ? (
                              <ChevronDown className="h-4 w-4" aria-hidden="true" />
                            ) : (
                              <ChevronRight className="h-4 w-4" aria-hidden="true" />
                            )}
                          </Button>
                        </TableCell>
                        <TableCell className="whitespace-nowrap align-top">
                          {change.time ? new Date(change.time).toLocaleString() : change.id}
                        </TableCell>
                        <TableCell className="align-top">
                          <span className="font-medium" title={change.actor || undefined}>
                            {change.actor ? shortName(change.actor) : '—'}
                          </span>
                        </TableCell>
                        <TableCell className="align-top">
                          <span
                            className="tone inline-block whitespace-nowrap rounded-full border px-2 py-1 text-xs font-medium"
                            style={toneStyle(TYPE_TONES[change.type] ?? 'slate')}
                          >
                            {change.type}
                          </span>
                        </TableCell>
                        <TableCell className="align-top">
                          <span className="max-w-xs break-all" title={change.dn}>{change.dn}</span>
                        </TableCell>
                        <TableCell className="hidden md:table-cell align-top">
                          <span
                            className={`inline-block whitespace-nowrap rounded-full border px-2 py-1 text-xs font-medium ${ORIGIN_CLASSES[change.origin]}`}
                          >
                            {ORIGIN_LABELS[change.origin]}
                          </span>
                        </TableCell>
                        <TableCell className="hidden md:table-cell align-top text-sm text-muted-foreground">
                          {summarise(change)}
                        </TableCell>
                      </TableRow>
                      {isOpen && (
                        <TableRow className="hover:bg-transparent">
                          <TableCell colSpan={7} className="bg-muted/30">
                            <ChangeDetails change={change} />
                          </TableCell>
                        </TableRow>
                      )}
                    </Fragment>
                  )
                })}
                {changes.length === 0 && (
                  <TableEmptyState
                    colSpan={7}
                    icon={DatabaseZap}
                    title={origin === 'external' && !type && !q ? 'Nothing written outside ldapwarden' : 'No writes match these filters'}
                    description={
                      origin === 'external'
                        ? 'Every write the directory logged in this window came through this app or the password policy. Choose "All writes" to see them.'
                        : 'Try a wider date range or another filter.'
                    }
                  >
                    {hasFilters && (
                      <Button variant="outline" size="sm" onClick={clearFilters}>
                        Clear filters
                      </Button>
                    )}
                  </TableEmptyState>
                )}
              </>
            )}
          </TableBody>
        </Table>
        {total > 0 && (
          <Pagination
            currentPage={currentPage}
            totalPages={totalPages}
            pageSize={pageSize}
            totalItems={total}
            onPageChange={(page) => onSearchChange({ page: page > 1 ? page : undefined }, false)}
            onPageSizeChange={(size) => onSearchChange({ size, page: undefined })}
          />
        )}
      </div>
    </div>
  )
}

function ChangeDetails({ change }: { change: DirectoryChange }) {
  return (
    <div className="space-y-4 py-2">
      {change.changes.length > 0 && (
        <div className="overflow-x-auto">
          <table className="w-full min-w-96 text-sm">
            <thead>
              <tr className="text-left text-xs uppercase text-muted-foreground">
                <th className="pb-1 pr-4 font-medium">Attribute</th>
                <th className="pb-1 pr-4 font-medium">Change</th>
                <th className="pb-1 font-medium">Values</th>
              </tr>
            </thead>
            <tbody>
              {change.changes.map((c, i) => (
                <tr key={`${c.attribute}-${c.op}-${i}`} className="border-t border-border/50 align-top">
                  <td className="whitespace-nowrap py-1 pr-4 font-medium">{c.attribute}</td>
                  <td className="whitespace-nowrap py-1 pr-4 text-muted-foreground">
                    {c.values.length === 0 && !c.redacted && (c.op === 'delete' || c.op === 'replace')
                      ? 'cleared'
                      : (OP_LABELS[c.op] ?? c.op)}
                  </td>
                  <td className="break-all py-1">
                    {c.redacted ? (
                      <span className="italic text-muted-foreground">secret, not shown</span>
                    ) : (
                      c.values.join(', ') || <span className="text-muted-foreground">—</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {change.old.length > 0 && (
        <div className="space-y-1">
          <p className="text-xs font-medium uppercase text-muted-foreground">
            {change.type === 'delete' ? 'The entry held' : 'Values before the change'}
          </p>
          <dl className="grid gap-x-4 gap-y-1 text-sm sm:grid-cols-[12rem_1fr]">
            {change.old.map((o) => (
              <div key={o.attribute} className="contents">
                <dt className="font-medium">{o.attribute}</dt>
                <dd className="break-all text-muted-foreground">
                  {o.redacted ? <span className="italic">secret, not shown</span> : o.values.join(', ')}
                </dd>
              </div>
            ))}
          </dl>
        </div>
      )}

      <dl className="grid gap-x-4 gap-y-1 text-sm sm:grid-cols-[8rem_1fr]">
        <div className="contents">
          <dt className="font-medium">Actor DN</dt>
          <dd className="break-all text-muted-foreground">{change.actor || '— (the server itself)'}</dd>
        </div>
        <div className="contents">
          <dt className="font-medium">Entry DN</dt>
          <dd className="break-all text-muted-foreground">{change.dn}</dd>
        </div>
        {change.type === 'modrdn' && (
          <div className="contents">
            <dt className="font-medium">Renamed to</dt>
            <dd className="break-all text-muted-foreground">
              {change.newRdn}
              {change.newSuperior ? ` under ${change.newSuperior}` : ''}
              {change.deleteOldRdn ? ' (old name removed)' : ''}
            </dd>
          </div>
        )}
        <div className="contents">
          <dt className="font-medium">Made</dt>
          <dd className="text-muted-foreground">
            {change.origin === 'ldapwarden' &&
              'Through ldapwarden: the app recorded a request by this identity at that moment, in the other tab.'}
            {change.origin === 'policy' &&
              'By the password policy, during a bind: failed attempts, the lockout they cause, or its lifting.'}
            {change.origin === 'external' &&
              'Outside ldapwarden: nothing in this app’s audit log accounts for it.'}
          </dd>
        </div>
      </dl>
    </div>
  )
}

// What to do when no access log is configured. The LDIF is the bundled stack's
// (ldap/config/accesslog.ldif), with the ACL naming the identity that has to
// read it.
function AccessLogSetup({ bindDn }: { bindDn: string }) {
  return (
    <div className="space-y-4">
      <Callout variant="info">
        No access log is configured, so only the changes made through ldapwarden are known — in the other tab.
        OpenLDAP can record every write itself with the <code>accesslog</code> overlay; point{' '}
        <code>LDAP_ACCESSLOG_BASE</code> at it and this tab lists the changes made with any other tool.
      </Callout>
      <p className="text-sm text-muted-foreground">
        On the directory, as the <code>cn=config</code> administrator (adjust the database numbers and the directory
        path to your server; the path must exist and belong to the slapd user):
      </p>
      <pre className="overflow-x-auto rounded-md border bg-muted p-3 text-xs">{accessLogLDIF(bindDn)}</pre>
      <p className="text-sm text-muted-foreground">
        Then set <code>LDAP_ACCESSLOG_BASE=cn=accesslog</code> and restart ldapwarden.
      </p>
    </div>
  )
}

function AccessLogUnreadable({ base, bindDn }: { base: string; bindDn: string }) {
  return (
    <div className="space-y-4">
      <Callout variant="warning">
        The directory has nothing to show at <code>{base}</code>. Either the accesslog overlay is not set up
        there, or <code>{bindDn}</code> is not allowed to read it — the directory gives the same answer to both, on
        purpose.
      </Callout>
      <p className="text-sm text-muted-foreground">
        If the overlay is in place, grant read access to the log database (adjust its number to your server):
      </p>
      <pre className="overflow-x-auto rounded-md border bg-muted p-3 text-xs">
        {`dn: olcDatabase={2}mdb,cn=config
changetype: modify
add: olcAccess
olcAccess: {0}to * by dn.exact="${bindDn}" read by * none`}
      </pre>
    </div>
  )
}

function accessLogLDIF(bindDn: string): string {
  return `dn: cn=module{0},cn=config
changetype: modify
add: olcModuleLoad
olcModuleLoad: accesslog

dn: olcDatabase={2}mdb,cn=config
changetype: add
objectClass: olcDatabaseConfig
objectClass: olcMdbConfig
olcDatabase: {2}mdb
olcSuffix: cn=accesslog
olcDbDirectory: /var/lib/ldap/accesslog
olcDbIndex: default eq
olcDbIndex: entryCSN,objectClass,reqEnd,reqResult,reqStart,reqDN eq
olcAccess: {0}to * by dn.exact="${bindDn}" read by * none

dn: olcOverlay=accesslog,olcDatabase={1}mdb,cn=config
changetype: add
objectClass: olcOverlayConfig
objectClass: olcAccessLogConfig
olcOverlay: accesslog
olcAccessLogDB: cn=accesslog
olcAccessLogOps: writes
olcAccessLogSuccess: TRUE
olcAccessLogOld: (objectClass=*)
olcAccessLogPurge: 07+00:00 01+00:00`
}
