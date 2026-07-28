import { InlineSpinner } from '@/components/inline-spinner'
import { createFileRoute, Link, redirect } from '@tanstack/react-router'
import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { api, userPhotoUrl } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
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
import { Plus, Search, CheckCircle2, Lock, Users, CalendarClock, Upload } from 'lucide-react'
import { SortIcon } from '@/components/ui/sort-icon'
import { useState, useMemo } from 'react'
import { encodeDN, parseLdapTimestamp } from '@/lib/utils'
import { assignTones, toneStyle } from '@/lib/tones'
import { useDebounced } from '@/lib/use-debounced'
import { Avatar } from '@/components/ui/avatar'
import { Callout } from '@/components/ui/callout'

type SortField = 'uid' | 'displayName' | 'mail' | 'employeeType' | 'groupsCount'
type SortDirection = 'asc' | 'desc'

// Helper to get account expiration date from either pwdAccountLockedTime or shadowExpire
function getAccountExpiration(user: { pwdAccountLockedTime?: string; shadowExpire?: number }): Date | null {
  // Check pwdAccountLockedTime first (LDAP generalized time format)
  if (user.pwdAccountLockedTime) {
    return parseLdapTimestamp(user.pwdAccountLockedTime)
  }
  // Check shadowExpire (days since Unix epoch, 0 or missing means no expiration)
  if (user.shadowExpire && user.shadowExpire > 0 && user.shadowExpire < 99999) {
    return new Date(user.shadowExpire * 24 * 60 * 60 * 1000)
  }
  return null
}

const DEFAULT_PAGE_SIZE = 25

export const Route = createFileRoute('/users/')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
  },
  component: UsersPage,
})

function UsersPage() {
  const { hasPermission } = useAuth()
  const [search, setSearch] = useState('')
  const [showInactive, setShowInactive] = useState(false)
  const [sortField, setSortField] = useState<SortField>('uid')
  const [sortDirection, setSortDirection] = useState<SortDirection>('asc')
  const [currentPage, setCurrentPage] = useState(1)
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const debouncedSearch = useDebounced(search)

  // Search, status filter, sort and paging are all applied by the directory
  // side, so a page costs one page worth of rows regardless of how large the
  // directory is.
  const { data, isLoading, isFetching, error } = useQuery({
    queryKey: ['users', { search: debouncedSearch, showInactive, sortField, sortDirection, currentPage, pageSize }],
    queryFn: ({ signal }) =>
      api.users.list(
        {
          search: debouncedSearch,
          status: showInactive ? undefined : 'active',
          sort: sortField,
          order: sortDirection,
          limit: pageSize,
          offset: (currentPage - 1) * pageSize,
        },
        signal,
      ),
    placeholderData: keepPreviousData,
  })

  // Group list for the "add to group" bulk action. Cheap now that the list
  // projection carries a member count instead of every member uid.
  const { data: groupsData } = useQuery({
    queryKey: ['groups', 'picker'],
    queryFn: ({ signal }) => api.groups.list(undefined, signal),
    enabled: hasPermission('users:write'),
    staleTime: 60_000,
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

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc')
    } else {
      setSortField(field)
      setSortDirection('asc')
    }
    setCurrentPage(1)
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
  const handleSearchChange = (value: string) => {
    setSearch(value)
    setCurrentPage(1)
    clearSelection()
  }

  const handlePageChange = (page: number) => {
    setCurrentPage(page)
    clearSelection()
  }

  const handlePageSizeChange = (size: number) => {
    setPageSize(size)
    setCurrentPage(1)
    clearSelection()
  }

  if (error) {
    return <Callout variant="error">Failed to load users: {error.message}</Callout>
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
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

      <div className="flex items-center gap-4">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
          <Input
            placeholder="Search users..."
            value={search}
            onChange={(e) => handleSearchChange(e.target.value)}
            className="pl-8"
          />
        </div>
        <label className="flex items-center gap-2 text-sm cursor-pointer select-none">
          <Switch
            checked={showInactive}
            onCheckedChange={(checked) => {
              setShowInactive(checked)
              setCurrentPage(1)
              clearSelection()
            }}
          />
          <span className="text-muted-foreground">
            Show inactive
            {inactiveCount > 0 && (
              <span className="ml-1 text-xs">({inactiveCount})</span>
            )}
          </span>
        </label>
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
          groups={(groupsData?.data ?? []).map((g) => ({ dn: g.dn, cn: g.cn }))}
          onClear={clearSelection}
        />
      )}

      {isLoading ? (
        <InlineSpinner />
      ) : (
        // Dim the table while a new page is in flight: keepPreviousData leaves
        // the old rows on screen, which would otherwise look like nothing
        // happened after a sort or page change.
        <div className={`border rounded-lg ${isFetching ? 'opacity-60 transition-opacity' : ''}`}>
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
                <TableHead>
                  <button
                    className="flex items-center hover:text-foreground"
                    onClick={() => handleSort('uid')}
                  >
                    Username
                    <SortIcon sortField={sortField} sortDirection={sortDirection} field="uid" />
                  </button>
                </TableHead>
                <TableHead>
                  <button
                    className="flex items-center hover:text-foreground"
                    onClick={() => handleSort('displayName')}
                  >
                    Name
                    <SortIcon sortField={sortField} sortDirection={sortDirection} field="displayName" />
                  </button>
                </TableHead>
                <TableHead>
                  <button
                    className="flex items-center hover:text-foreground"
                    onClick={() => handleSort('mail')}
                  >
                    Email
                    <SortIcon sortField={sortField} sortDirection={sortDirection} field="mail" />
                  </button>
                </TableHead>
                <TableHead>
                  <button
                    className="flex items-center hover:text-foreground"
                    onClick={() => handleSort('employeeType')}
                  >
                    Type
                    <SortIcon sortField={sortField} sortDirection={sortDirection} field="employeeType" />
                  </button>
                </TableHead>
                <TableHead>
                  <button
                    className="flex items-center hover:text-foreground"
                    onClick={() => handleSort('groupsCount')}
                  >
                    Groups
                    <SortIcon sortField={sortField} sortDirection={sortDirection} field="groupsCount" />
                  </button>
                </TableHead>
                <TableHead className="w-16 text-center">Status</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
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
                  <TableCell className="text-center">
                    {(() => {
                      const expDate = getAccountExpiration(user)
                      const now = new Date()
                      const isExpired = expDate && expDate < now
                      const hasFutureExpiration = expDate && expDate >= now

                      // A locked account and a lapsed expiration are the same
                      // underlying state (a past pwdAccountLockedTime blocks
                      // login), so both get the red padlock; the tooltip keeps
                      // the nuance where we can tell them apart.
                      if (user.accountLocked || isExpired) {
                        return (
                          <span title={user.accountLocked ? 'Account locked' : 'Expired'}>
                            <Lock className="h-4 w-4 inline-block text-destructive" />
                          </span>
                        )
                      } else if (hasFutureExpiration) {
                        // Calculate days until expiration for color coding
                        const daysUntil = Math.ceil((expDate.getTime() - now.getTime()) / (1000 * 60 * 60 * 24))

                        let colorClass = 'text-success' // > 30 days
                        const title = `Expires in ${daysUntil} days`

                        if (daysUntil <= 7) {
                          colorClass = 'text-destructive'
                        } else if (daysUntil <= 30) {
                          colorClass = 'text-warning'
                        }

                        return (
                          <span title={title}>
                            <CalendarClock className={`h-4 w-4 inline-block ${colorClass}`} />
                          </span>
                        )
                      } else {
                        return (
                          <span title="Active">
                            <CheckCircle2 className="h-4 w-4 inline-block text-success" />
                          </span>
                        )
                      }
                    })()}
                  </TableCell>
                </TableRow>
              ))}
              {users.length === 0 && (
                <TableRow>
                  <TableCell colSpan={canWrite ? 7 : 6} className="text-center py-8 text-muted-foreground">
                    No users found
                  </TableCell>
                </TableRow>
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
      )}
    </div>
  )
}
