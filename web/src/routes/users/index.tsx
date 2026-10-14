import { TableSkeletonRows } from '@/components/ui/skeleton'
import { useRememberListSearch } from '@/lib/list-memory'
import { createFileRoute, Link, redirect } from '@tanstack/react-router'
import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { api, userPhotoUrl } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Select } from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Pagination } from '@/components/ui/pagination'
import { Checkbox } from '@/components/ui/checkbox'
import { UsersBulkActions } from '@/components/users-bulk-actions'
import { CheckCircle2, Plus, Search, Users, Upload } from 'lucide-react'
import { SortableHeader } from '@/components/ui/sortable-header'
import { useCallback, useMemo, useState, useRef } from 'react'
import { encodeDN } from '@/lib/utils'
import { assignTones, toneStyle } from '@/lib/tones'
import { useUrlSearchInput } from '@/lib/use-url-search-input'
import { useSearchHotkey } from '@/lib/use-hotkey'
import {
  listSearchValidator,
  listSearchValues,
  nextSort,
  asBoolean,
  type ListSearch,
} from '@/lib/list-search'
import { Avatar } from '@/components/ui/avatar'
import { AccountStatus } from '@/components/account-status'
import { TableEmptyState } from '@/components/ui/empty-state'
import { Callout } from '@/components/ui/callout'
import { requireModule } from '@/lib/use-server-info'

const SORT_FIELDS = ['uid', 'displayName', 'mail', 'employeeType', 'groupsCount'] as const
type SortField = (typeof SORT_FIELDS)[number]

// Which accounts the list shows. Absent means active only, the everyday view;
// `inactive` is the locked-or-expired set the dashboard's card links to.
const STATUS_FILTERS = ['all', 'inactive'] as const
type StatusFilter = (typeof STATUS_FILTERS)[number]

// UsersSearch is the view state this page keeps in the URL, so a filtered,
// sorted, paged view is a link someone can bookmark or paste into a ticket.
interface UsersSearch extends ListSearch<SortField> {
  status?: StatusFilter
}

function parseStatus(raw: Record<string, unknown>): StatusFilter | undefined {
  if (typeof raw.status === 'string' && (STATUS_FILTERS as readonly string[]).includes(raw.status)) {
    return raw.status as StatusFilter
  }
  // `?inactive=true` was the "Show inactive" toggle; links made with it still
  // mean "everyone".
  return asBoolean(raw.inactive) ? 'all' : undefined
}

export const Route = createFileRoute('/users/')({
  beforeLoad: async ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
    await requireModule(context.queryClient, 'users')
  },
  validateSearch: (raw: Record<string, unknown>): UsersSearch => ({
    ...listSearchValidator(SORT_FIELDS)(raw),
    status: parseStatus(raw),
  }),
  component: UsersPage,
})

function UsersPage() {
  const { hasPermission } = useAuth()
  const searchParams = Route.useSearch()
  useRememberListSearch('/users', searchParams)
  const navigate = Route.useNavigate()
  const { q, sort: sortField, dir: sortDirection, page: currentPage, size: pageSize } =
    listSearchValues<SortField>(searchParams, 'uid')
  const statusFilter = searchParams.status
  const [selected, setSelected] = useState<Set<string>>(new Set())


  // patchSearch is the single writer of this page's view state. `replace` keeps
  // filter tweaks out of the history stack — only page moves are worth a Back.
  const patchSearch = useCallback(
    (patch: Partial<UsersSearch>, replace = true) => {
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

  // Search, status filter, sort and paging are all applied by the directory
  // side, so a page costs one page worth of rows regardless of how large the
  // directory is.
  const { data, isLoading, isFetching, error } = useQuery({
    queryKey: ['users', { search: q, statusFilter, sortField, sortDirection, currentPage, pageSize }],
    queryFn: ({ signal }) =>
      api.users.list(
        {
          search: q,
          status: statusFilter === 'all' ? undefined : (statusFilter ?? 'active'),
          sort: sortField,
          order: sortDirection,
          limit: pageSize,
          offset: (currentPage - 1) * pageSize,
        },
        signal,
      ),
    placeholderData: keepPreviousData,
  })

  // Memoised so the derived selection/tone maps below keep a stable identity
  // between renders.
  const users = useMemo(() => data?.data ?? [], [data?.data])
  const total = data?.total ?? 0
  const inactiveCount = data?.inactiveCount ?? 0
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  // Assign a stable tint to each employee type present on this page.
  const employeeTypeTones = useMemo(
    () => assignTones(users.map((u) => u.employeeType).filter((t): t is string => !!t)),
    [users],
  )

  const canWrite = hasPermission('users:write')
  const canDelete = hasPermission('users:delete')

  const handleSort = (field: SortField) => {
    patchSearch(nextSort({ sort: sortField, dir: sortDirection }, field))
    clearSelection()
  }

  // Selection is keyed by DN and only spans the visible page: with paging done
  // server-side the rest of the result set is not in memory, and a bulk action
  // must never operate on rows the operator has not seen.
  const selectedUsers = useMemo(() => users.filter((u) => selected.has(u.dn)), [users, selected])
  const allSelected = users.length > 0 && users.every((u) => selected.has(u.dn))
  const someSelected = selected.size > 0 && !allSelected

  const toggleOne = (dn: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(dn)) next.delete(dn)
      else next.add(dn)
      return next
    })
  }
  const toggleAll = () => {
    setSelected((prev) => (users.every((u) => prev.has(u.dn)) ? new Set() : new Set(users.map((u) => u.dn))))
  }
  const clearSelection = () => setSelected(new Set())

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

  const handleStatusChange = (status: StatusFilter | undefined) => {
    patchSearch({ status, page: undefined })
    clearSelection()
  }

  // The header stays when the list fails to load: the page is still the
  // page, and creating an entry does not depend on listing them.
  const header = (
    <div className="flex flex-wrap items-center justify-between gap-2">
      <h1 className="text-2xl font-bold">Users</h1>
      {canWrite && (
        <div className="flex items-center gap-2">
          <Link to="/users/import">
            <Button variant="outline">
              <Upload className="h-4 w-4 mr-1" />
              Import CSV
            </Button>
          </Link>
          <Link to="/users/new">
            <Button>
              <Plus className="h-4 w-4 mr-1" />
              New User
            </Button>
          </Link>
        </div>
      )}
    </div>
  )

  if (error) {
    return (
      <div className="space-y-4">
        {header}
        <Callout variant="error">Failed to load users: {error.message}</Callout>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {header}

      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <div className="relative min-w-48 flex-1 max-w-sm">
          <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
          <Input
            ref={searchRef}
            placeholder="Search users..."
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="pl-8"
          />
        </div>
        <Select
          aria-label="Account status"
          className="w-auto"
          value={statusFilter ?? 'active'}
          onChange={(e) => {
            const value = e.target.value
            handleStatusChange(value === 'active' ? undefined : (value as StatusFilter))
          }}
          options={[
            { value: 'active', label: 'Active' },
            { value: 'inactive', label: `Locked or expired${inactiveCount > 0 ? ` (${inactiveCount})` : ''}` },
            { value: 'all', label: 'All accounts' },
          ]}
        />
        <span className="text-sm text-muted-foreground">
          {total} user{total !== 1 ? 's' : ''}
        </span>
      </div>

      {data?.truncated && (
        <Callout variant="warning">
          Showing the first {data.total} matches. Refine your search to narrow the results.
        </Callout>
      )}

      {canWrite && (
        <UsersBulkActions
          selected={selectedUsers}
          onClear={clearSelection}
          canDelete={canDelete}
        />
      )}

      {/* Dim the table while a refetch is in flight: keepPreviousData leaves the
          old rows on screen, which would otherwise look like nothing happened
          after a sort or page change. */}
      <div className={`border rounded-lg ${isFetching && !isLoading ? 'opacity-60 transition-opacity' : ''}`}>
        <Table>
          <TableHeader>
            <TableRow>
              {canWrite && (
                <TableHead className="w-10">
                  <Checkbox
                    aria-label="Select all users"
                    checked={allSelected}
                    indeterminate={someSelected}
                    onCheckedChange={toggleAll}
                  />
                </TableHead>
              )}
              <SortableHeader
                field="uid"
                activeField={sortField}
                direction={sortDirection}
                onSort={handleSort}
              >
                Username
              </SortableHeader>
              <SortableHeader
                field="displayName"
                activeField={sortField}
                direction={sortDirection}
                onSort={handleSort}
              >
                Name
              </SortableHeader>
              <SortableHeader
                field="mail"
                activeField={sortField}
                direction={sortDirection}
                onSort={handleSort}
              >
                Email
              </SortableHeader>
              <SortableHeader
                field="employeeType"
                activeField={sortField}
                direction={sortDirection}
                onSort={handleSort}
              >
                Type
              </SortableHeader>
              <SortableHeader
                field="groupsCount"
                activeField={sortField}
                direction={sortDirection}
                onSort={handleSort}
              >
                Groups
              </SortableHeader>
              <TableHead className="w-36">Status</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableSkeletonRows rows={Math.min(pageSize, 8)} columns={canWrite ? 7 : 6} />
            ) : (
              <>
                {users.map((user) => (
              <TableRow key={user.dn} data-state={selected.has(user.dn) ? 'selected' : undefined}>
                {canWrite && (
                  <TableCell className="w-10">
                    <Checkbox
                      aria-label={`Select ${user.uid}`}
                      checked={selected.has(user.dn)}
                      onCheckedChange={() => toggleOne(user.dn)}
                    />
                  </TableCell>
                )}
                <TableCell>
                  <Link
                    to="/users/$dn"
                    params={{ dn: encodeDN(user.dn) }}
                    className="flex items-center gap-2 hover:underline"
                  >
                    <Avatar
                      url={user.hasPhoto ? userPhotoUrl(user.dn) : undefined}
                      fallback={user.displayName || user.cn}
                      size="sm"
                    />
                    <span className="font-medium">{user.uid}</span>
                  </Link>
                </TableCell>
                <TableCell>{user.displayName || user.cn}</TableCell>
                <TableCell>{user.mail || '-'}</TableCell>
                <TableCell>
                  {user.employeeType ? (
                    <Badge
                      className="tone"
                      style={toneStyle(employeeTypeTones.get(user.employeeType) ?? 'slate')}
                    >
                      {user.employeeType}
                    </Badge>
                  ) : (
                    <span className="text-muted-foreground">-</span>
                  )}
                </TableCell>
                <TableCell>
                  <div className="flex items-center gap-1">
                    <Users className="h-3.5 w-3.5 text-muted-foreground" />
                    <span>{user.groupsCount ?? 0}</span>
                  </div>
                </TableCell>
                <TableCell>
                  <AccountStatus user={user} withText />
                </TableCell>
              </TableRow>
            ))}
            {users.length === 0 && (
              q ? (
                <TableEmptyState
                  colSpan={canWrite ? 7 : 6}
                  icon={Search}
                  title={`No users match \u201c${q}\u201d`}
                  description="Search matches uid, name and email."
                >
                  <Button variant="outline" size="sm" onClick={() => setSearchInput('')}>
                    Clear search
                  </Button>
                </TableEmptyState>
              ) : statusFilter === 'inactive' ? (
                // The good outcome of the dashboard's "Locked or expired" link.
                <TableEmptyState
                  colSpan={canWrite ? 7 : 6}
                  icon={CheckCircle2}
                  title="No locked or expired accounts"
                  description="Every account can currently log in."
                >
                  <Button variant="outline" size="sm" onClick={() => handleStatusChange(undefined)}>
                    Show active accounts
                  </Button>
                </TableEmptyState>
              ) : statusFilter === undefined && inactiveCount > 0 ? (
                <TableEmptyState
                  colSpan={canWrite ? 7 : 6}
                  icon={Search}
                  title="No active users"
                  description="Every account is locked or expired."
                >
                  <Button variant="outline" size="sm" onClick={() => handleStatusChange('inactive')}>
                    Show locked or expired
                  </Button>
                </TableEmptyState>
              ) : (
                <TableEmptyState
                  colSpan={canWrite ? 7 : 6}
                  icon={Users}
                  title="No users yet"
                  description={
                    canWrite
                      ? 'Create one, or import a CSV to populate the directory.'
                      : 'The directory has no user entries under the configured OU.'
                  }
                >
                  {canWrite && (
                    <>
                      <Link to="/users/new">
                        <Button size="sm">
                          <Plus className="h-4 w-4 mr-1" />
                          New User
                        </Button>
                      </Link>
                      <Link to="/users/import">
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
