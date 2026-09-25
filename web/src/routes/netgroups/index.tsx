import { TableSkeletonRows } from '@/components/ui/skeleton'
import { useRememberListSearch } from '@/lib/list-memory'
import { createFileRoute, Link, redirect } from '@tanstack/react-router'
import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { api, type Netgroup } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import { BulkDeleteActions } from '@/components/bulk-delete-actions'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Pagination } from '@/components/ui/pagination'
import { SortableHeader } from '@/components/ui/sortable-header'
import { Callout } from '@/components/ui/callout'
import { TableEmptyState } from '@/components/ui/empty-state'
import { Plus, Search, Network, Server, User, AlertTriangle, Upload } from 'lucide-react'
import { useCallback, useMemo, useRef } from 'react'
import { encodeDN } from '@/lib/utils'
import { useUrlSearchInput } from '@/lib/use-url-search-input'
import { useSearchHotkey } from '@/lib/use-hotkey'
import { useRowSelection } from '@/lib/use-row-selection'
import { netgroupKind, netgroupKindLabel } from '@/lib/netgroup'
import {
  listSearchValidator,
  listSearchValues,
  nextSort,
  type ListSearch,
} from '@/lib/list-search'

const SORT_FIELDS = ['cn'] as const
type SortField = (typeof SORT_FIELDS)[number]

export const Route = createFileRoute('/netgroups/')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
  },
  validateSearch: listSearchValidator(SORT_FIELDS),
  component: NetgroupsPage,
})

function NetgroupsPage() {
  const { hasPermission } = useAuth()
  const searchParams = Route.useSearch()
  useRememberListSearch('/netgroups', searchParams)
  const navigate = Route.useNavigate()
  const { q, sort: sortField, dir: sortDirection, page: currentPage, size: pageSize } =
    listSearchValues<SortField>(searchParams, 'cn')

  const patchSearch = useCallback(
    (patch: Partial<ListSearch<SortField>>, replace = true) => {
      navigate({ search: (prev) => ({ ...prev, ...patch }), replace })
    },
    [navigate],
  )

  const [searchInput, setSearchInput] = useUrlSearchInput(
    q,
    useCallback((value: string) => patchSearch({ q: value || undefined, page: undefined }), [patchSearch]),
  )

  const searchRef = useRef<HTMLInputElement>(null)
  useSearchHotkey(searchRef)

  const { data, isLoading, isFetching, error } = useQuery({
    queryKey: ['netgroups', { search: q, sortField, sortDirection, currentPage, pageSize }],
    queryFn: ({ signal }) =>
      api.netgroups.list(
        {
          search: q,
          sort: sortField,
          order: sortDirection,
          limit: pageSize,
          offset: (currentPage - 1) * pageSize,
        },
        signal,
      ),
    placeholderData: keepPreviousData,
  })

  const netgroups = useMemo(() => data?.data ?? [], [data?.data])
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  const canWrite = hasPermission('groups:write')
  const canDelete = hasPermission('groups:delete')

  const {
    selected,
    selectedRows,
    allSelected,
    someSelected,
    toggleOne,
    toggleAll,
    clear: clearSelection,
  } = useRowSelection(netgroups)

  const handleSort = (field: SortField) => {
    patchSearch(nextSort({ sort: sortField, dir: sortDirection }, field))
    clearSelection()
  }

  const handlePageChange = (page: number) => {
    patchSearch({ page: page > 1 ? page : undefined }, false)
    clearSelection()
  }

  const handlePageSizeChange = (size: number) => {
    patchSearch({ size, page: undefined })
    clearSelection()
  }

  if (error) {
    return <Callout variant="error">Failed to load netgroups: {error.message}</Callout>
  }

  const columns = canDelete ? 5 : 4

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-2xl font-bold">Netgroups</h1>
        {canWrite && (
          <div className="flex items-center gap-2">
            <Link to="/netgroups/import">
              <Button variant="outline">
                <Upload className="h-4 w-4 mr-1" />
                Import CSV
              </Button>
            </Link>
            <Link to="/netgroups/new">
              <Button>
                <Plus className="h-4 w-4 mr-1" />
                New Netgroup
              </Button>
            </Link>
          </div>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-48 flex-1 max-w-sm">
          <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
          <Input
            ref={searchRef}
            placeholder="Search netgroups..."
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="pl-8"
          />
        </div>
        <span className="text-sm text-muted-foreground">
          {total} netgroup{total !== 1 ? 's' : ''}
        </span>
      </div>

      {data?.truncated && (
        <Callout variant="warning">
          Showing the first {data.total} matches. Refine your search to narrow the results.
        </Callout>
      )}

      {canDelete && (
        <BulkDeleteActions
          selected={selectedRows}
          onClear={clearSelection}
          noun="netgroup"
          label={(netgroup) => netgroup.cn}
          remove={(netgroup) => api.netgroups.delete(netgroup.dn)}
          invalidateKey={['netgroups']}
        />
      )}

      <div className={`border rounded-lg ${isFetching && !isLoading ? 'opacity-60 transition-opacity' : ''}`}>
        <Table>
          <TableHeader>
            <TableRow>
              {canDelete && (
                <TableHead className="w-10">
                  <Checkbox
                    aria-label="Select all netgroups"
                    checked={allSelected}
                    indeterminate={someSelected}
                    onCheckedChange={toggleAll}
                  />
                </TableHead>
              )}
              <SortableHeader
                field="cn"
                activeField={sortField}
                direction={sortDirection}
                onSort={handleSort}
              >
                Name
              </SortableHeader>
              <TableHead>Description</TableHead>
              <TableHead>Contents</TableHead>
              <TableHead>Nests</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableSkeletonRows rows={Math.min(pageSize, 8)} columns={columns} />
            ) : (
              <>
                {netgroups.map((netgroup) => (
                  <TableRow
                    key={netgroup.dn}
                    data-state={selected.has(netgroup.dn) ? 'selected' : undefined}
                  >
                    {canDelete && (
                      <TableCell className="w-10">
                        <Checkbox
                          aria-label={`Select ${netgroup.cn}`}
                          checked={selected.has(netgroup.dn)}
                          onCheckedChange={() => toggleOne(netgroup.dn)}
                        />
                      </TableCell>
                    )}
                    <TableCell>
                      <Link
                        to="/netgroups/$dn"
                        params={{ dn: encodeDN(netgroup.dn) }}
                        className="flex items-center gap-2 hover:underline"
                      >
                        <Network className="h-4 w-4 text-muted-foreground" aria-hidden="true" />
                        <span className="font-medium">{netgroup.cn}</span>
                      </Link>
                    </TableCell>
                    <TableCell>{netgroup.description || '-'}</TableCell>
                    <TableCell>
                      <ContentsCell netgroup={netgroup} />
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {netgroup.members?.length
                        ? netgroup.members.join(', ')
                        : '-'}
                    </TableCell>
                  </TableRow>
                ))}
                {netgroups.length === 0 && (
                  q ? (
                    <TableEmptyState
                      colSpan={columns}
                      icon={Search}
                      title={`No netgroups match “${q}”`}
                      description="Search matches the name, the description, the triples and nested names."
                    >
                      <Button variant="outline" size="sm" onClick={() => setSearchInput('')}>
                        Clear search
                      </Button>
                    </TableEmptyState>
                  ) : (
                    <TableEmptyState
                      colSpan={columns}
                      icon={Network}
                      title="No netgroups yet"
                      description={
                        canWrite
                          ? 'A netgroup collects users or hosts, and a sudo rule can grant to all of them at once with "+name".'
                          : 'The directory has no nisNetgroup entries under the configured OU.'
                      }
                    >
                      {canWrite && (
                        <>
                          <Link to="/netgroups/new">
                            <Button size="sm">
                              <Plus className="h-4 w-4 mr-1" />
                              New Netgroup
                            </Button>
                          </Link>
                          <Link to="/netgroups/import">
                            <Button variant="outline" size="sm">
                              <Upload className="h-4 w-4 mr-1" />
                              Import CSV
                            </Button>
                          </Link>
                        </>
                      )}
                    </TableEmptyState>
                  )
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
            onPageChange={handlePageChange}
            onPageSizeChange={handlePageSizeChange}
          />
        )}
      </div>
    </div>
  )
}

/**
 * What the netgroup holds, at a glance.
 *
 * Counts rather than names: the useful question in a list is which side a
 * netgroup can be used from, since a netgroup of users named in sudoHost — or
 * the reverse — matches nothing at all and says nothing about it.
 */
function ContentsCell({ netgroup }: { netgroup: Netgroup }) {
  const kind = netgroupKind(netgroup)
  const users = netgroup.triples.filter((t) => t.user && t.user !== '-').length
  const hosts = netgroup.triples.filter((t) => t.host && t.host !== '-').length

  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
      {users > 0 && (
        <span className="flex items-center gap-1">
          <User className="h-3.5 w-3.5 text-muted-foreground" aria-hidden="true" />
          {users}
        </span>
      )}
      {hosts > 0 && (
        <span className="flex items-center gap-1">
          <Server className="h-3.5 w-3.5 text-muted-foreground" aria-hidden="true" />
          {hosts}
        </span>
      )}
      {netgroup.triples.length === 0 && (
        <span className="text-muted-foreground">
          {netgroup.members?.length ? 'nested only' : 'empty'}
        </span>
      )}
      {kind === 'mixed' && (
        <span className="flex items-center gap-1 text-warning" title={netgroupKindLabel(kind)}>
          <AlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />
          mixed
        </span>
      )}
      {netgroup.malformed?.length ? (
        <span
          className="flex items-center gap-1 text-warning"
          title="Values the directory holds that this app cannot read. They are preserved untouched."
        >
          <AlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />
          {netgroup.malformed.length} unreadable
        </span>
      ) : null}
    </div>
  )
}
