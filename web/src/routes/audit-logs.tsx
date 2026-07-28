import { InlineSpinner } from '@/components/inline-spinner'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
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
import { Callout } from '@/components/ui/callout'
import { toneStyle, type Tone } from '@/lib/tones'
import { DEFAULT_PAGE_SIZE, PAGE_SIZES } from '@/lib/list-search'

// Filter options. Values match the backend's resource-type and action strings
// exactly (see internal/audit/audit.go); labels mirror them so the dropdown
// reads the same as the action badge in the table.
const RESOURCE_TYPE_OPTIONS = [
  { value: '', label: 'All resource types' },
  { value: 'user', label: 'User' },
  { value: 'group', label: 'Group' },
  { value: 'sudorole', label: 'Sudo role' },
  { value: 'pwdpolicy', label: 'Password policy' },
  { value: 'schema', label: 'Schema' },
]

const ACTION_OPTIONS = [
  { value: '', label: 'All actions' },
  { value: 'login', label: 'login' },
  { value: 'login.failed', label: 'login.failed' },
  { value: 'logout', label: 'logout' },
  { value: 'user.create', label: 'user.create' },
  { value: 'user.update', label: 'user.update' },
  { value: 'user.delete', label: 'user.delete' },
  { value: 'user.lock', label: 'user.lock' },
  { value: 'user.unlock', label: 'user.unlock' },
  { value: 'group.create', label: 'group.create' },
  { value: 'group.update', label: 'group.update' },
  { value: 'group.delete', label: 'group.delete' },
  { value: 'group.member.add', label: 'group.member.add' },
  { value: 'group.member.remove', label: 'group.member.remove' },
  { value: 'sudorole.create', label: 'sudorole.create' },
  { value: 'sudorole.update', label: 'sudorole.update' },
  { value: 'sudorole.delete', label: 'sudorole.delete' },
  { value: 'pwdpolicy.create', label: 'pwdpolicy.create' },
  { value: 'pwdpolicy.update', label: 'pwdpolicy.update' },
  { value: 'pwdpolicy.delete', label: 'pwdpolicy.delete' },
  { value: 'schema.refresh', label: 'schema.refresh' },
]

// AuditSearch is the view state kept in the URL, so a filtered view of the log
// — the thing you actually want to send to someone — is a link.
interface AuditSearch {
  resourceType?: string
  action?: string
  page?: number
  size?: number
}

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
    resourceType: RESOURCE_TYPE_OPTIONS.some((o) => o.value !== '' && o.value === raw.resourceType)
      ? (raw.resourceType as string)
      : undefined,
    action: ACTION_OPTIONS.some((o) => o.value !== '' && o.value === raw.action)
      ? (raw.action as string)
      : undefined,
    page: Number.isInteger(Number(raw.page)) && Number(raw.page) > 1 ? Number(raw.page) : undefined,
    size: (PAGE_SIZES as readonly number[]).includes(Number(raw.size)) && Number(raw.size) !== DEFAULT_PAGE_SIZE
      ? Number(raw.size)
      : undefined,
  }),
  component: AuditLogsPage,
})

function AuditLogsPage() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const resourceType = search.resourceType ?? ''
  const action = search.action ?? ''
  const currentPage = search.page ?? 1
  const pageSize = search.size ?? DEFAULT_PAGE_SIZE

  const hasFilters = resourceType !== '' || action !== ''

  const patchSearch = (patch: Partial<AuditSearch>, replace = true) => {
    navigate({ search: (prev) => ({ ...prev, ...patch }), replace })
  }

  const { data, isLoading, error } = useQuery({
    queryKey: ['audit-logs', currentPage, pageSize, resourceType, action],
    queryFn: ({ signal }) =>
      api.auditLogs.list(
        {
          limit: pageSize,
          offset: (currentPage - 1) * pageSize,
          resourceType: resourceType || undefined,
          action: action || undefined,
        },
        signal,
      ),
  })

  const totalPages = data ? Math.ceil(data.total / pageSize) : 0

  const handlePageChange = (page: number) => {
    patchSearch({ page: page > 1 ? page : undefined }, false)
  }

  const handlePageSizeChange = (size: number) => {
    patchSearch({ size, page: undefined })
  }

  // Changing a filter must reset to the first page, otherwise the current page
  // index could exceed the filtered result's page count and show nothing.
  const handleResourceTypeChange = (value: string) => {
    patchSearch({ resourceType: value || undefined, page: undefined })
  }
  const handleActionChange = (value: string) => {
    patchSearch({ action: value || undefined, page: undefined })
  }
  const clearFilters = () => {
    patchSearch({ resourceType: undefined, action: undefined, page: undefined })
  }

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleString()
  }

  // Tint an action by what it does to the directory, mirroring the colours the
  // audit notification emails already use.
  const actionTone = (action: string): Tone => {
    if (action.includes('create')) return 'emerald'
    if (action.includes('delete')) return 'rose'
    if (action.includes('update')) return 'blue'
    if (action.includes('login')) return 'violet'
    return 'slate'
  }

  if (error) {
    return (
      <Callout variant="error">
        Failed to load audit logs: {error.message}
      </Callout>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Audit Logs</h1>
        <span className="text-sm text-muted-foreground">
          {data?.total ?? 0}{hasFilters ? ' matching' : ' total'} entries
        </span>
      </div>

      <div className="flex flex-wrap items-end gap-3">
        <div className="space-y-1">
          <label className="text-xs font-medium text-muted-foreground">Resource type</label>
          <Select
            className="w-48"
            options={RESOURCE_TYPE_OPTIONS}
            value={resourceType}
            onChange={(e) => handleResourceTypeChange(e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <label className="text-xs font-medium text-muted-foreground">Action</label>
          <Select
            className="w-56"
            options={ACTION_OPTIONS}
            value={action}
            onChange={(e) => handleActionChange(e.target.value)}
          />
        </div>
        {hasFilters && (
          <Button variant="ghost" size="sm" onClick={clearFilters}>
            Clear filters
          </Button>
        )}
      </div>

      {isLoading ? (
        <InlineSpinner />
      ) : (
        <>
          <div className="border rounded-lg">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Timestamp</TableHead>
                  <TableHead>Actor</TableHead>
                  <TableHead>Action</TableHead>
                  <TableHead>Resource</TableHead>
                  <TableHead>Details</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data?.data.map((log) => (
                  <TableRow key={log.id}>
                    <TableCell className="whitespace-nowrap">
                      {formatDate(log.createdAt)}
                    </TableCell>
                    <TableCell>
                      <span className="font-medium" title={log.actorDn || undefined}>{log.actorUid}</span>
                    </TableCell>
                    <TableCell>
                      <span
                        className="tone inline-block rounded-full border px-2 py-1 text-xs font-medium"
                        style={toneStyle(actionTone(log.action))}
                      >
                        {log.action}
                      </span>
                    </TableCell>
                    <TableCell>
                      <div className="max-w-xs truncate" title={log.resourceDn || undefined}>
                        <span className="text-muted-foreground">{log.resourceType}: </span>
                        {log.resourceDn || '-'}
                      </div>
                    </TableCell>
                    <TableCell>
                      {log.details && Object.keys(log.details).length > 0 ? (
                        <code
                          className="block max-w-xs truncate text-xs bg-muted px-1 py-0.5 rounded"
                          title={JSON.stringify(log.details, null, 2)}
                        >
                          {JSON.stringify(log.details)}
                        </code>
                      ) : (
                        '-'
                      )}
                    </TableCell>
                  </TableRow>
                ))}
                {data?.data.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={5} className="text-center py-8 text-muted-foreground">
                      {hasFilters ? 'No audit logs match these filters' : 'No audit logs found'}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
            {data && data.total > 0 && (
              <Pagination
                currentPage={currentPage}
                totalPages={totalPages}
                pageSize={pageSize}
                totalItems={data.total}
                onPageChange={handlePageChange}
                onPageSizeChange={handlePageSizeChange}
              />
            )}
          </div>
        </>
      )}
    </div>
  )
}
