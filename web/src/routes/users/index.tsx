import { InlineSpinner } from '@/components/inline-spinner'
import { createFileRoute, Link, redirect } from '@tanstack/react-router'
import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { api } from '@/lib/api'
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
import { Plus, Search, CheckCircle2, Circle, Users, CalendarClock, Upload } from 'lucide-react'
import { SortIcon } from '@/components/ui/sort-icon'
import { useState, useMemo } from 'react'
import { encodeDN, parseLdapTimestamp } from '@/lib/utils'
import { useDebounced } from '@/lib/use-debounced'
import { Avatar } from '@/components/ui/avatar'

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

// Color palette for employee type badges
const BADGE_COLORS = [
  'bg-blue-100 text-blue-800 border-blue-200',
  'bg-green-100 text-green-800 border-green-200',
  'bg-purple-100 text-purple-800 border-purple-200',
  'bg-orange-100 text-orange-800 border-orange-200',
  'bg-pink-100 text-pink-800 border-pink-200',
  'bg-cyan-100 text-cyan-800 border-cyan-200',
  'bg-yellow-100 text-yellow-800 border-yellow-200',
  'bg-red-100 text-red-800 border-red-200',
  'bg-indigo-100 text-indigo-800 border-indigo-200',
  'bg-teal-100 text-teal-800 border-teal-200',
]

// Build a color map from unique employee types
function buildColorMap(types: string[]): Map<string, string> {
  const uniqueTypes = [...new Set(types)].sort()
  const colorMap = new Map<string, string>()
  uniqueTypes.forEach((type, index) => {
    colorMap.set(type, BADGE_COLORS[index % BADGE_COLORS.length])
  })
  return colorMap
}

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

  const { data, isLoading, error } = useQuery({
    queryKey: ['users', debouncedSearch],
    queryFn: ({ signal }) => api.users.list(debouncedSearch, signal),
    placeholderData: keepPreviousData,
  })

  const { data: groupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: ({ signal }) => api.groups.list(undefined, signal),
  })

  // Build a map of uid -> number of groups
  const groupsRawData = groupsData?.data
  const userGroupsCount = useMemo(() => {
    const countMap: Record<string, number> = {}
    if (groupsRawData) {
      for (const group of groupsRawData) {
        for (const memberUid of group.memberUid || []) {
          countMap[memberUid] = (countMap[memberUid] || 0) + 1
        }
      }
    }
    return countMap
  }, [groupsRawData])

  // Build color map for employee types
  const employeeTypeColors = useMemo(() => {
    const types = data?.data
      .map(user => user.employeeType)
      .filter((type): type is string => !!type) ?? []
    return buildColorMap(types)
  }, [data?.data])

  const canWrite = hasPermission('users:write')

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc')
    } else {
      setSortField(field)
      setSortDirection('asc')
    }
  }

  const { sortedUsers, allUsers, totalFiltered, totalPages, inactiveCount } = useMemo(() => {
    // Search is applied server-side; only the active-status filter remains
    // client-side. Account is inactive if locked OR expired.
    const filtered = data?.data.filter((user) => {
      const expDate = getAccountExpiration(user)
      const isExpired = expDate && expDate < new Date()
      const isInactive = user.accountLocked || isExpired
      return showInactive || !isInactive
    }) ?? []

    // Count inactive users for display
    const inactiveCount = data?.data.filter(u => {
      const expDate = getAccountExpiration(u)
      const isExpired = expDate && expDate < new Date()
      return u.accountLocked || isExpired
    }).length ?? 0

    const sorted = [...filtered].sort((a, b) => {
      let aVal: string | number = ''
      let bVal: string | number = ''

      switch (sortField) {
        case 'uid':
          aVal = a.uid.toLowerCase()
          bVal = b.uid.toLowerCase()
          break
        case 'displayName':
          aVal = (a.displayName || a.cn).toLowerCase()
          bVal = (b.displayName || b.cn).toLowerCase()
          break
        case 'mail':
          aVal = (a.mail || '').toLowerCase()
          bVal = (b.mail || '').toLowerCase()
          break
        case 'employeeType':
          aVal = (a.employeeType || '').toLowerCase()
          bVal = (b.employeeType || '').toLowerCase()
          break
        case 'groupsCount':
          aVal = userGroupsCount[a.uid] || 0
          bVal = userGroupsCount[b.uid] || 0
          break
      }

      if (aVal < bVal) return sortDirection === 'asc' ? -1 : 1
      if (aVal > bVal) return sortDirection === 'asc' ? 1 : -1
      return 0
    })

    const totalPages = Math.ceil(sorted.length / pageSize)
    const startIndex = (currentPage - 1) * pageSize
    const paginatedUsers = sorted.slice(startIndex, startIndex + pageSize)

    return {
      sortedUsers: paginatedUsers,
      allUsers: sorted,
      totalFiltered: sorted.length,
      totalPages,
      inactiveCount,
    }
  }, [data?.data, showInactive, sortField, sortDirection, currentPage, pageSize, userGroupsCount])

  // Selection helpers. Selection is keyed by DN and spans the whole filtered
  // set (not just the visible page), so "select all" and bulk actions operate
  // on every matching user.
  const selectedUsers = useMemo(
    () => allUsers.filter((u) => selected.has(u.dn)),
    [allUsers, selected],
  )
  const allSelected = allUsers.length > 0 && allUsers.every((u) => selected.has(u.dn))
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
    setSelected((prev) => (allUsers.every((u) => prev.has(u.dn)) ? new Set() : new Set(allUsers.map((u) => u.dn))))
  }
  const clearSelection = () => setSelected(new Set())

  // Reset to first page when search changes
  const handleSearchChange = (value: string) => {
    setSearch(value)
    setCurrentPage(1)
  }

  const handlePageSizeChange = (size: number) => {
    setPageSize(size)
    setCurrentPage(1)
  }

  if (error) {
    return (
      <div className="p-4 text-destructive bg-destructive/10 rounded-md">
        Failed to load users: {error.message}
      </div>
    )
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
          {totalFiltered} user{totalFiltered !== 1 ? 's' : ''}
        </span>
      </div>

      {data?.truncated && (
        <div className="rounded-md border border-yellow-500/40 bg-yellow-500/10 px-3 py-2 text-sm text-yellow-700 dark:text-yellow-500">
          Showing the first {data.total} matches. Refine your search to narrow the results.
        </div>
      )}

      {canWrite && (
        <UsersBulkActions
          selected={selectedUsers}
          groups={(groupsRawData ?? []).map((g) => ({ dn: g.dn, cn: g.cn }))}
          onClear={clearSelection}
        />
      )}

      {isLoading ? (
        <InlineSpinner />
      ) : (
        <div className="border rounded-lg">
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
              {sortedUsers.map((user) => (
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
                        src={user.jpegPhoto}
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
                      <Badge className={employeeTypeColors.get(user.employeeType)}>{user.employeeType}</Badge>
                    ) : (
                      <span className="text-muted-foreground">-</span>
                    )}
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1">
                      <Users className="h-3.5 w-3.5 text-muted-foreground" />
                      <span>{userGroupsCount[user.uid] || 0}</span>
                    </div>
                  </TableCell>
                  <TableCell className="text-center">
                    {(() => {
                      const expDate = getAccountExpiration(user)
                      const now = new Date()
                      const isExpired = expDate && expDate < now
                      const hasFutureExpiration = expDate && expDate >= now

                      if (isExpired) {
                        return (
                          <span title="Expired">
                            <Circle className="h-4 w-4 inline-block text-muted-foreground" />
                          </span>
                        )
                      } else if (user.accountLocked) {
                        return (
                          <span title="Account disabled">
                            <Circle className="h-4 w-4 inline-block text-muted-foreground" />
                          </span>
                        )
                      } else if (hasFutureExpiration) {
                        // Calculate days until expiration for color coding
                        const daysUntil = Math.ceil((expDate.getTime() - now.getTime()) / (1000 * 60 * 60 * 24))

                        let colorClass = 'text-green-500' // > 30 days
                        const title = `Expires in ${daysUntil} days`

                        if (daysUntil <= 7) {
                          colorClass = 'text-red-500'
                        } else if (daysUntil <= 30) {
                          colorClass = 'text-amber-400'
                        }

                        return (
                          <span title={title}>
                            <CalendarClock className={`h-4 w-4 inline-block ${colorClass}`} />
                          </span>
                        )
                      } else {
                        return (
                          <span title="Active">
                            <CheckCircle2 className="h-4 w-4 inline-block text-green-500" />
                          </span>
                        )
                      }
                    })()}
                  </TableCell>
                </TableRow>
              ))}
              {sortedUsers.length === 0 && (
                <TableRow>
                  <TableCell colSpan={canWrite ? 7 : 6} className="text-center py-8 text-muted-foreground">
                    No users found
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
          {totalFiltered > 0 && (
            <Pagination
              currentPage={currentPage}
              totalPages={totalPages}
              pageSize={pageSize}
              totalItems={totalFiltered}
              onPageChange={setCurrentPage}
              onPageSizeChange={handlePageSizeChange}
            />
          )}
        </div>
      )}
    </div>
  )
}
