import { createFileRoute, Link, redirect } from '@tanstack/react-router'
import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { api, FieldChangeSchema, type AuditLog, type AuditLogParams, type FieldChange } from '@/lib/api'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Pagination } from '@/components/ui/pagination'
import { Select } from '@/components/ui/select'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { DatePicker } from '@/components/ui/date-picker'
import { Callout } from '@/components/ui/callout'
import { TableEmptyState } from '@/components/ui/empty-state'
import { TableSkeletonRows } from '@/components/ui/skeleton'
import { actionTone, toneStyle } from '@/lib/tones'
import { DEFAULT_PAGE_SIZE, PAGE_SIZES } from '@/lib/list-search'
import { useUrlSearchInput } from '@/lib/use-url-search-input'
import { useSearchHotkey } from '@/lib/use-hotkey'
import { resourceLink } from '@/lib/audit-links'
import { ChevronDown, ChevronRight, Download, ExternalLink, ScrollText, Search } from 'lucide-react'
import { Fragment, useCallback, useState, useRef } from 'react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { DirectoryChangesView } from '@/components/directory-changes'
import {
  CHANGE_TYPE_OPTIONS,
  ORIGIN_OPTIONS,
  type DirectoryChangesSearch,
  type OriginFilter,
} from '@/lib/directory-changes'

// Filter options. Values match the backend's resource-type and action strings
// exactly (see internal/audit/audit.go); labels mirror them so the dropdown
// reads the same as the action badge in the table.
const RESOURCE_TYPE_OPTIONS = [
  { value: '', label: 'All resource types' },
  { value: 'user', label: 'User' },
  { value: 'group', label: 'Group' },
  { value: 'sudorole', label: 'Sudo role' },
  { value: 'pwdpolicy', label: 'Password policy' },
  { value: 'netgroup', label: 'Netgroup' },
  { value: 'schema', label: 'Schema' },
  { value: 'directory', label: 'Directory' },
]

const ACTION_OPTIONS = [
  { value: '', label: 'All actions' },
  { value: 'login', label: 'login' },
  { value: 'login.failed', label: 'login.failed' },
  { value: 'logout', label: 'logout' },
  { value: 'user.create', label: 'user.create' },
  { value: 'user.update', label: 'user.update' },
  { value: 'user.delete', label: 'user.delete' },
  { value: 'user.rename', label: 'user.rename' },
  { value: 'user.lock', label: 'user.lock' },
  { value: 'user.unlock', label: 'user.unlock' },
  // The directory locking an account by itself, found by the lockout scan — as
  // opposed to user.lock, which is an administrator doing it here.
  { value: 'user.lockout', label: 'user.lockout' },
  { value: 'group.create', label: 'group.create' },
  { value: 'group.update', label: 'group.update' },
  { value: 'group.delete', label: 'group.delete' },
  { value: 'group.rename', label: 'group.rename' },
  { value: 'group.member.add', label: 'group.member.add' },
  { value: 'group.member.remove', label: 'group.member.remove' },
  { value: 'sudorole.create', label: 'sudorole.create' },
  { value: 'sudorole.update', label: 'sudorole.update' },
  { value: 'sudorole.delete', label: 'sudorole.delete' },
  { value: 'sudorole.user.add', label: 'sudorole.user.add' },
  { value: 'sudorole.user.remove', label: 'sudorole.user.remove' },
  { value: 'sudorole.group.add', label: 'sudorole.group.add' },
  { value: 'sudorole.group.remove', label: 'sudorole.group.remove' },
  { value: 'netgroup.create', label: 'netgroup.create' },
  { value: 'netgroup.update', label: 'netgroup.update' },
  { value: 'netgroup.delete', label: 'netgroup.delete' },
  { value: 'pwdpolicy.create', label: 'pwdpolicy.create' },
  { value: 'pwdpolicy.update', label: 'pwdpolicy.update' },
  { value: 'pwdpolicy.delete', label: 'pwdpolicy.delete' },
  { value: 'schema.refresh', label: 'schema.refresh' },
  { value: 'policy.selftest', label: 'policy.selftest' },
  // An LDIF download: not a change, but it hands over every password hash.
  { value: 'directory.export', label: 'directory.export' },
]

// AuditSearch is the view state kept in the URL, so a filtered slice of the log
// — the thing you actually want to hand to someone — is a link.
interface AuditSearch extends DirectoryChangesSearch {
  // Which log: this app's own (default), or the directory's (accesslog).
  view?: 'directory'
  actor?: string
  resourceType?: string
  action?: string
  q?: string
  from?: string
  to?: string
  page?: number
  size?: number
}

const ISO_DAY = /^\d{4}-\d{2}-\d{2}$/

export const Route = createFileRoute('/audit-logs')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
    if (!context.auth.hasPermission('audit:read')) {
      throw redirect({ to: '/' })
    }
  },
  validateSearch: (raw: Record<string, unknown>): AuditSearch => ({
    view: raw.view === 'directory' ? 'directory' : undefined,
    // 'external' is the directory view's default, so it is left out of the URL.
    origin: ORIGIN_OPTIONS.some((o) => o.value !== 'external' && o.value === raw.origin)
      ? (raw.origin as OriginFilter)
      : undefined,
    type: CHANGE_TYPE_OPTIONS.some((o) => o.value !== '' && o.value === raw.type) ? (raw.type as string) : undefined,
    actor: typeof raw.actor === 'string' && raw.actor !== '' ? raw.actor : undefined,
    resourceType: RESOURCE_TYPE_OPTIONS.some((o) => o.value !== '' && o.value === raw.resourceType)
      ? (raw.resourceType as string)
      : undefined,
    action: ACTION_OPTIONS.some((o) => o.value !== '' && o.value === raw.action)
      ? (raw.action as string)
      : undefined,
    q: typeof raw.q === 'string' && raw.q !== '' ? raw.q : undefined,
    from: typeof raw.from === 'string' && ISO_DAY.test(raw.from) ? raw.from : undefined,
    to: typeof raw.to === 'string' && ISO_DAY.test(raw.to) ? raw.to : undefined,
    page: Number.isInteger(Number(raw.page)) && Number(raw.page) > 1 ? Number(raw.page) : undefined,
    size: (PAGE_SIZES as readonly number[]).includes(Number(raw.size)) && Number(raw.size) !== DEFAULT_PAGE_SIZE
      ? Number(raw.size)
      : undefined,
  }),
  component: AuditLogsPage,
})


// changesOf pulls the structured diff out of details. The handlers store it
// under details.changes; anything else in details is shown separately.
function changesOf(log: AuditLog): FieldChange[] {
  const raw = log.details?.changes
  if (!Array.isArray(raw)) return []
  return raw.flatMap((item) => {
    const parsed = FieldChangeSchema.safeParse(item)
    return parsed.success ? [parsed.data] : []
  })
}

function otherDetails(log: AuditLog): Array<[string, unknown]> {
  return Object.entries(log.details ?? {}).filter(
    ([key]) => key !== 'changes' && key !== 'resourceName',
  )
}

// resourceLabel prefers the human-readable name the handlers record over the raw
// DN, falling back to the DN (and to a dash for entries with no resource).
function resourceLabel(log: AuditLog): string {
  const name = log.details?.resourceName
  if (typeof name === 'string' && name !== '') return name
  return log.resourceDn || '-'
}

// summarise is the one-line preview in the collapsed row: which fields changed,
// or the shape of whatever else the action recorded.
function summarise(changes: FieldChange[], extras: Array<[string, unknown]>): string {
  if (changes.length > 0) {
    const fields = changes.map((c) => c.field).join(', ')
    return changes.length <= 3 ? fields : `${changes.length} fields: ${fields}`
  }
  return extras.length > 0 ? extras.map(([key]) => key).join(', ') : '—'
}

function AuditLogsPage() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const view = search.view ?? 'app'

  // Filters do not carry over between the two logs: they do not mean the same
  // thing in both.
  const setView = (next: string) => {
    navigate({ search: next === 'directory' ? { view: 'directory' } : {}, replace: true })
  }
  const patchSearch = useCallback(
    (patch: Partial<AuditSearch>, replace = true) => {
      navigate({ search: (prev) => ({ ...prev, ...patch }), replace })
    },
    [navigate],
  )

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-bold">Audit Logs</h1>
      <Tabs value={view} onValueChange={setView}>
        <TabsList>
          <TabsTrigger value="app">Through ldapwarden</TabsTrigger>
          <TabsTrigger value="directory">In the directory</TabsTrigger>
        </TabsList>
        <TabsContent value="app">
          <AppAuditLog />
        </TabsContent>
        <TabsContent value="directory">
          <DirectoryChangesView search={search} onSearchChange={patchSearch} />
        </TabsContent>
      </Tabs>
    </div>
  )
}

// AppAuditLog is what this app recorded: every request it served that changed
// something, with who asked, from where, and the field-level diff.
function AppAuditLog() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const actor = search.actor ?? ''
  const resourceType = search.resourceType ?? ''
  const action = search.action ?? ''
  const from = search.from ?? ''
  const to = search.to ?? ''
  const q = search.q ?? ''
  const currentPage = search.page ?? 1
  const pageSize = search.size ?? DEFAULT_PAGE_SIZE

  // Rows the operator has opened, keyed by entry id so paging back and forth
  // keeps what was expanded.
  const [expanded, setExpanded] = useState<Set<string>>(new Set())

  const hasFilters = Boolean(actor || resourceType || action || from || to || q)

  const patchSearch = useCallback(
    (patch: Partial<AuditSearch>, replace = true) => {
      navigate({ search: (prev) => ({ ...prev, ...patch }), replace })
    },
    [navigate],
  )

  const [searchInput, setSearchInput] = useUrlSearchInput(
    q,
    useCallback((value: string) => patchSearch({ q: value || undefined, page: undefined }), [patchSearch]),
  )

  // `/` focuses the search box, the convention in keyboard-driven tools.
  const searchRef = useRef<HTMLInputElement>(null)
  useSearchHotkey(searchRef)

  // The filters shared by the listing and the export, so a downloaded CSV always
  // covers exactly the view it was taken from.
  const filters: AuditLogParams = {
    actorUid: actor || undefined,
    resourceType: resourceType || undefined,
    action: action || undefined,
    search: q || undefined,
    from: from || undefined,
    to: to || undefined,
  }

  const { data, isLoading, isFetching, error } = useQuery({
    queryKey: ['audit-logs', { ...filters, currentPage, pageSize }],
    queryFn: ({ signal }) =>
      api.auditLogs.list({ ...filters, limit: pageSize, offset: (currentPage - 1) * pageSize }, signal),
    placeholderData: keepPreviousData,
  })

  const { data: actorsData } = useQuery({
    queryKey: ['audit-logs', 'actors'],
    queryFn: ({ signal }) => api.auditLogs.actors(signal),
    staleTime: 5 * 60 * 1000,
  })

  const knownActors = actorsData?.data ?? []
  const actorOptions = [
    { value: '', label: 'All actors' },
    // Keep a filtered actor selectable even when they have dropped off the
    // recent-actors list, so a shared link never silently loses its filter.
    ...(actor && !knownActors.includes(actor) ? [{ value: actor, label: actor }] : []),
    ...knownActors.map((uid) => ({ value: uid, label: uid })),
  ]

  const logs = data?.data ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

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
    patchSearch({
      actor: undefined,
      resourceType: undefined,
      action: undefined,
      q: undefined,
      from: undefined,
      to: undefined,
      page: undefined,
    })
  }

  if (error) {
    return <Callout variant="error">Failed to load audit logs: {error.message}</Callout>
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-end gap-2">
        <div className="flex items-center gap-3">
          <span className="text-sm text-muted-foreground">
            {total}{hasFilters ? ' matching' : ' total'} entries
          </span>
          {/* A plain download link rather than a fetch: the browser streams the
              CSV straight from the endpoint, so a large export never has to sit
              in JS memory. */}
          <a href={api.auditLogs.exportUrl(filters)} download>
            <Button variant="outline" size="sm">
              <Download className="h-4 w-4 mr-1" />
              Export CSV
            </Button>
          </a>
        </div>
      </div>

      <div className="flex flex-wrap items-end gap-3">
        <div className="space-y-1">
          <label htmlFor="audit-search" className="text-xs font-medium text-muted-foreground">
            Actor or resource
          </label>
          <div className="relative">
            <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input
              id="audit-search"
              ref={searchRef}
              className="w-56 pl-8"
              placeholder="uid or DN fragment…"
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
            />
          </div>
        </div>
        <div className="space-y-1">
          <label htmlFor="audit-actor" className="text-xs font-medium text-muted-foreground">Actor</label>
          <Select
            id="audit-actor"
            className="w-40"
            options={actorOptions}
            value={actor}
            onChange={(e) => patchSearch({ actor: e.target.value || undefined, page: undefined })}
          />
        </div>
        <div className="space-y-1">
          <label htmlFor="audit-resource" className="text-xs font-medium text-muted-foreground">Resource type</label>
          <Select
            id="audit-resource"
            className="w-44"
            options={RESOURCE_TYPE_OPTIONS}
            value={resourceType}
            onChange={(e) => patchSearch({ resourceType: e.target.value || undefined, page: undefined })}
          />
        </div>
        <div className="space-y-1">
          <label htmlFor="audit-action" className="text-xs font-medium text-muted-foreground">Action</label>
          <Select
            id="audit-action"
            className="w-52"
            options={ACTION_OPTIONS}
            value={action}
            onChange={(e) => patchSearch({ action: e.target.value || undefined, page: undefined })}
          />
        </div>
        <div className="space-y-1">
          <label htmlFor="audit-from" className="text-xs font-medium text-muted-foreground">From</label>
          <DatePicker
            id="audit-from"
            className="w-40"
            value={from}
            max={to || undefined}
            onChange={(value) => patchSearch({ from: value || undefined, page: undefined })}
          />
        </div>
        <div className="space-y-1">
          <label htmlFor="audit-to" className="text-xs font-medium text-muted-foreground">To</label>
          <DatePicker
            id="audit-to"
            className="w-40"
            value={to}
            min={from || undefined}
            onChange={(value) => patchSearch({ to: value || undefined, page: undefined })}
          />
        </div>
        {hasFilters && (
          <Button variant="ghost" size="sm" onClick={clearFilters}>
            Clear filters
          </Button>
        )}
      </div>

      {/* Dim the table while a refetch is in flight: keepPreviousData leaves the
          old rows on screen, which would otherwise look like nothing happened. */}
      <div className={`border rounded-lg ${isFetching && !isLoading ? 'opacity-60 transition-opacity' : ''}`}>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-10" />
              <TableHead>Timestamp</TableHead>
              <TableHead>Actor</TableHead>
              <TableHead>Action</TableHead>
              <TableHead>Resource</TableHead>
              {/* Below md the row keeps who, what and when; the change is one tap
                  away in the expanded details. */}
              <TableHead className="hidden md:table-cell">Change</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableSkeletonRows rows={Math.min(pageSize, 8)} columns={6} hideBelowMd={[5]} />
            ) : (
              <>
                {logs.map((log) => {
                  const isOpen = expanded.has(log.id)
                  const changes = changesOf(log)
                  const extras = otherDetails(log)
                  const link = resourceLink(log)

                  return (
                    <Fragment key={log.id}>
                      <TableRow>
                        <TableCell className="w-10 align-top">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-6 w-6"
                            aria-expanded={isOpen}
                            aria-label={isOpen ? 'Hide details' : 'Show details'}
                            onClick={() => toggleExpanded(log.id)}
                          >
                            {isOpen ? (
                              <ChevronDown className="h-4 w-4" aria-hidden="true" />
                            ) : (
                              <ChevronRight className="h-4 w-4" aria-hidden="true" />
                            )}
                          </Button>
                        </TableCell>
                        <TableCell className="whitespace-nowrap align-top">
                          {new Date(log.createdAt).toLocaleString()}
                        </TableCell>
                        <TableCell className="align-top">
                          <span className="font-medium" title={log.actorDn || undefined}>{log.actorUid}</span>
                        </TableCell>
                        <TableCell className="align-top">
                          <span
                            className="tone inline-block whitespace-nowrap rounded-full border px-2 py-1 text-xs font-medium"
                            style={toneStyle(actionTone(log.action))}
                          >
                            {log.action}
                          </span>
                        </TableCell>
                        <TableCell className="align-top">
                          <div className="max-w-xs">
                            <span className="text-muted-foreground">{log.resourceType}: </span>
                            {link ? (
                              <Link
                                to={link.to}
                                params={{ dn: link.dn }}
                                className="inline-flex items-center gap-1 break-all hover:underline"
                              >
                                {resourceLabel(log)}
                                <ExternalLink className="h-3 w-3 shrink-0" aria-hidden="true" />
                              </Link>
                            ) : (
                              <span className="break-all">{resourceLabel(log)}</span>
                            )}
                          </div>
                        </TableCell>
                        <TableCell className="hidden md:table-cell align-top text-sm text-muted-foreground">
                          {summarise(changes, extras)}
                        </TableCell>
                      </TableRow>
                      {isOpen && (
                        <TableRow className="hover:bg-transparent">
                          <TableCell colSpan={6} className="bg-muted/30">
                            <EntryDetails log={log} changes={changes} extras={extras} />
                          </TableCell>
                        </TableRow>
                      )}
                    </Fragment>
                  )
                })}
                {logs.length === 0 && (
                  <TableEmptyState
                    colSpan={6}
                    icon={ScrollText}
                    title={hasFilters ? 'No entries match these filters' : 'No audit entries yet'}
                    description={
                      hasFilters
                        ? 'The log is not empty — try a wider date range, resource type or action.'
                        : 'Every change made through the UI is recorded here, along with the actor and their IP.'
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
            onPageChange={(page) => patchSearch({ page: page > 1 ? page : undefined }, false)}
            onPageSizeChange={(size) => patchSearch({ size, page: undefined })}
          />
        )}
      </div>
    </div>
  )
}

/**
 * The expanded row: the diff as a real before/after table, plus the request
 * metadata. All of this used to be one JSON blob truncated into a `title`
 * attribute, which made the one thing an audit log is for — seeing what actually
 * changed — unreadable.
 */
function EntryDetails({
  log,
  changes,
  extras,
}: {
  log: AuditLog
  changes: FieldChange[]
  extras: Array<[string, unknown]>
}) {
  return (
    <div className="space-y-4 py-2">
      {changes.length > 0 && (
        <div className="overflow-x-auto">
          <table className="w-full min-w-96 text-sm">
            <thead>
              <tr className="text-left text-xs uppercase text-muted-foreground">
                <th className="pb-1 pr-4 font-medium">Field</th>
                <th className="pb-1 pr-4 font-medium">Before</th>
                <th className="pb-1 font-medium">After</th>
              </tr>
            </thead>
            <tbody>
              {changes.map((change) => (
                <tr key={change.field} className="border-t border-border/50 align-top">
                  <td className="whitespace-nowrap py-1 pr-4 font-medium">{change.field}</td>
                  {change.masked ? (
                    <td className="py-1 italic text-muted-foreground" colSpan={2}>
                      value not recorded
                    </td>
                  ) : (
                    <>
                      <td className="break-all py-1 pr-4 text-destructive">
                        {change.old || <span className="text-muted-foreground">(empty)</span>}
                      </td>
                      <td className="break-all py-1 text-success">
                        {change.new || <span className="text-muted-foreground">(empty)</span>}
                      </td>
                    </>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <dl className="grid gap-x-4 gap-y-1 text-sm sm:grid-cols-[8rem_1fr]">
        {extras.map(([key, value]) => (
          <div key={key} className="contents">
            <dt className="font-medium">{key}</dt>
            <dd className="break-all text-muted-foreground">
              {typeof value === 'string' ? value : JSON.stringify(value)}
            </dd>
          </div>
        ))}
        <div className="contents">
          <dt className="font-medium">Actor DN</dt>
          <dd className="break-all text-muted-foreground">{log.actorDn || '—'}</dd>
        </div>
        <div className="contents">
          <dt className="font-medium">Resource DN</dt>
          <dd className="break-all text-muted-foreground">{log.resourceDn || '—'}</dd>
        </div>
        <div className="contents">
          <dt className="font-medium">IP address</dt>
          <dd className="text-muted-foreground">{log.ipAddress || '—'}</dd>
        </div>
        <div className="contents">
          <dt className="font-medium">User agent</dt>
          <dd className="break-all text-muted-foreground">{log.userAgent || '—'}</dd>
        </div>
      </dl>
    </div>
  )
}
