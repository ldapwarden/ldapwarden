import { TableSkeletonRows } from '@/components/ui/skeleton'
import { useRememberListSearch } from '@/lib/list-memory'
import { createFileRoute, Link, redirect } from '@tanstack/react-router'
import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { api } from '@/lib/api'
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
import { Plus, Search, Users, Upload } from 'lucide-react'
import { SortableHeader } from '@/components/ui/sortable-header'
import { Callout } from '@/components/ui/callout'
import { TableEmptyState } from '@/components/ui/empty-state'
import { useCallback, useMemo, useRef } from 'react'
import { encodeDN } from '@/lib/utils'
import { useUrlSearchInput } from '@/lib/use-url-search-input'
import { useSearchHotkey } from '@/lib/use-hotkey'
import { useRowSelection } from '@/lib/use-row-selection'
import {
  listSearchValidator,
  listSearchValues,
  nextSort,
  type ListSearch,
} from '@/lib/list-search'

const SORT_FIELDS = ['cn', 'description', 'gidNumber', 'members'] as const
type SortField = (typeof SORT_FIELDS)[number]

export const Route = createFileRoute('/groups/')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
  },
  validateSearch: listSearchValidator(SORT_FIELDS),
  component: GroupsPage,
})

function GroupsPage() {
  const { hasPermission } = useAuth()
  const searchParams = Route.useSearch()
  useRememberListSearch('/groups', searchParams)
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

  // `/` focuses the search box, the convention in keyboard-driven tools.
  const searchRef = useRef<HTMLInputElement>(null)
  useSearchHotkey(searchRef)

  // Search, sort and paging are applied by the directory side; the browser
  // renders one page of a light projection.
  const { data, isLoading, isFetching, error } = useQuery({
    queryKey: ['groups', { search: q, sortField, sortDirection, currentPage, pageSize }],
    queryFn: ({ signal }) =>
      api.groups.list(
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

  // Memoised so the derived selection below keeps a stable identity between
  // renders.
  const groups = useMemo(() => data?.data ?? [], [data?.data])
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  const canWrite = hasPermission('groups:write')
  // Selection exists for the bulk delete alone, so it follows the permission
  // DELETE /api/groups/:dn requires, not the one that creates groups.
  const canDelete = hasPermission('groups:delete')

  const {
    selected,
    selectedRows: selectedGroups,
    allSelected,
    someSelected,
    toggleOne,
    toggleAll,
    clear: clearSelection,
  } = useRowSelection(groups)

  const handleSort = (field: SortField) => {
    patchSearch(nextSort({ sort: sortField, dir: sortDirection }, field))
    clearSelection()
  }

  // Any change to the query resets paging and selection: page 3 of the old
  // result set is meaningless, and a selection carried across it would apply to
  // rows that are no longer on screen.
  const handlePageChange = (page: number) => {
    // Paging is the one move worth a history entry, so Back returns to the
    // previous page instead of leaving the list entirely.
    patchSearch({ page: page > 1 ? page : undefined }, false)
    clearSelection()
  }

  const handlePageSizeChange = (size: number) => {
    patchSearch({ size, page: undefined })
    clearSelection()
  }

  if (error) {
    return (
      <Callout variant="error">
        Failed to load groups: {error.message}
      </Callout>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-2xl font-bold">Groups</h1>
        {canWrite && (
          <div className="flex items-center gap-2">
            <Link to="/groups/import">
              <Button variant="outline">
                <Upload className="h-4 w-4 mr-1" />
                Import CSV
              </Button>
            </Link>
            <Link to="/groups/new">
              <Button>
                <Plus className="h-4 w-4 mr-1" />
                New Group
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
            placeholder="Search groups..."
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="pl-8"
          />
        </div>
        <span className="text-sm text-muted-foreground">
          {total} group{total !== 1 ? 's' : ''}
        </span>
      </div>

      {data?.truncated && (
        <Callout variant="warning">
          Showing the first {data.total} matches. Refine your search to narrow the results.
        </Callout>
      )}

      {canDelete && (
        <BulkDeleteActions
          selected={selectedGroups}
          onClear={clearSelection}
          noun="group"
          label={(group) => group.cn}
          remove={(group) => api.groups.delete(group.dn)}
          invalidateKey={['groups']}
        />
      )}

      {/* Dim the table while a refetch is in flight: keepPreviousData leaves the
          old rows on screen, which would otherwise look like nothing happened
          after a sort or page change. */}
      <div className={`border rounded-lg ${isFetching ? 'opacity-60 transition-opacity' : ''}`}>
        <Table>
          <TableHeader>
            <TableRow>
              {canDelete && (
                <TableHead className="w-10">
                  <Checkbox
                    aria-label="Select all groups"
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
              <SortableHeader
                field="description"
                activeField={sortField}
                direction={sortDirection}
                onSort={handleSort}
              >
                Description
              </SortableHeader>
              <SortableHeader
                field="gidNumber"
                activeField={sortField}
                direction={sortDirection}
                onSort={handleSort}
              >
                GID
              </SortableHeader>
              <SortableHeader
                field="members"
                activeField={sortField}
                direction={sortDirection}
                onSort={handleSort}
              >
                Members
              </SortableHeader>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableSkeletonRows rows={Math.min(pageSize, 8)} columns={canDelete ? 5 : 4} />
            ) : (
              <>
                {groups.map((group) => (
              <TableRow key={group.dn} data-state={selected.has(group.dn) ? 'selected' : undefined}>
                {canDelete && (
                  <TableCell className="w-10">
                    <Checkbox
                      aria-label={`Select ${group.cn}`}
                      checked={selected.has(group.dn)}
                      onCheckedChange={() => toggleOne(group.dn)}
                    />
                  </TableCell>
                )}
                <TableCell>
                  <Link
                    to="/groups/$dn"
                    params={{ dn: encodeDN(group.dn) }}
                    className="font-medium hover:underline"
                  >
                    {group.cn}
                  </Link>
                </TableCell>
                <TableCell>{group.description || '-'}</TableCell>
                <TableCell>{group.gidNumber}</TableCell>
                <TableCell>
                  <div className="flex items-center gap-1">
                    <Users className="h-4 w-4 text-muted-foreground" />
                    {group.membersCount ?? 0}
                  </div>
                </TableCell>
              </TableRow>
            ))}
            {groups.length === 0 && (
              q ? (
                <TableEmptyState
                  colSpan={canDelete ? 5 : 4}
                  icon={Search}
                  title={`No groups match \u201c${q}\u201d`}
                  description="Search matches the group name and description."
                >
                  <Button variant="outline" size="sm" onClick={() => setSearchInput('')}>
                    Clear search
                  </Button>
                </TableEmptyState>
              ) : (
                <TableEmptyState
                  colSpan={canDelete ? 5 : 4}
                  icon={Users}
                  title="No groups yet"
                  description={
                    canWrite
                      ? 'Create one, or import a CSV to populate the directory.'
                      : 'The directory has no posixGroup entries under the configured OU.'
                  }
                >
                  {canWrite && (
                    <>
                      <Link to="/groups/new">
                        <Button size="sm">
                          <Plus className="h-4 w-4 mr-1" />
                          New Group
                        </Button>
                      </Link>
                      <Link to="/groups/import">
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
