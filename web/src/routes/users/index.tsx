import { createFileRoute, Link, redirect } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
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
import { Plus, Search, ArrowUp, ArrowDown, ArrowUpDown, CheckCircle2, Circle, Users, CalendarClock } from 'lucide-react'
import { useState, useMemo } from 'react'
import { encodeDN, parseLdapTimestamp } from '@/lib/utils'
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

  const { data, isLoading, error } = useQuery({
    queryKey: ['users'],
    queryFn: api.users.list,
  })

  const { data: groupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: api.groups.list,
  })

  // Build a map of uid -> number of groups
  const userGroupsCount = useMemo(() => {
    const countMap: Record<string, number> = {}
    if (groupsData?.data) {
      for (const group of groupsData.data) {
        for (const memberUid of group.memberUid || []) {
          countMap[memberUid] = (countMap[memberUid] || 0) + 1
        }
      }
    }
    return countMap
  }, [groupsData?.data])

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

  const SortIcon = ({ field }: { field: SortField }) => {
    if (sortField !== field) return <ArrowUpDown className="ml-1 h-4 w-4 text-muted-foreground" />
    return sortDirection === 'asc'
      ? <ArrowUp className="ml-1 h-4 w-4" />
      : <ArrowDown className="ml-1 h-4 w-4" />
  }

  const { sortedUsers, totalFiltered, totalPages, inactiveCount } = useMemo(() => {
    const filtered = data?.data.filter((user) => {
      // Filter by active status
      // Account is inactive if locked OR if expiration date is in the past
      const expDate = getAccountExpiration(user)
      const isExpired = expDate && expDate < new Date()
      const isInactive = user.accountLocked || isExpired
      if (!showInactive && isInactive) {
        return false
      }

      // Filter by search
      const searchLower = search.toLowerCase()
      return (
        user.uid.toLowerCase().includes(searchLower) ||
        user.cn.toLowerCase().includes(searchLower) ||
        user.mail?.toLowerCase().includes(searchLower) ||
        user.displayName?.toLowerCase().includes(searchLower)
      )
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
      totalFiltered: sorted.length,
      totalPages,
      inactiveCount,
    }
  }, [data?.data, search, showInactive, sortField, sortDirection, currentPage, pageSize, userGroupsCount])

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
          <Link to="/users/new">
            <Button>
              <Plus className="h-4 w-4 mr-1" />
              New User
            </Button>
          </Link>
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

      {isLoading ? (
        <div className="flex items-center justify-center py-8">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
        </div>
      ) : (
        <div className="border rounded-lg">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>
                  <button
                    className="flex items-center hover:text-foreground"
                    onClick={() => handleSort('uid')}
                  >
                    Username
                    <SortIcon field="uid" />
                  </button>
                </TableHead>
                <TableHead>
                  <button
                    className="flex items-center hover:text-foreground"
                    onClick={() => handleSort('displayName')}
                  >
                    Name
                    <SortIcon field="displayName" />
                  </button>
                </TableHead>
                <TableHead>
                  <button
                    className="flex items-center hover:text-foreground"
                    onClick={() => handleSort('mail')}
                  >
                    Email
                    <SortIcon field="mail" />
                  </button>
                </TableHead>
                <TableHead>
                  <button
                    className="flex items-center hover:text-foreground"
                    onClick={() => handleSort('employeeType')}
                  >
                    Type
                    <SortIcon field="employeeType" />
                  </button>
                </TableHead>
                <TableHead>
                  <button
                    className="flex items-center hover:text-foreground"
                    onClick={() => handleSort('groupsCount')}
                  >
                    Groups
                    <SortIcon field="groupsCount" />
                  </button>
                </TableHead>
                <TableHead className="w-16 text-center">Status</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sortedUsers.map((user) => (
                <TableRow key={user.dn}>
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
                  <TableCell colSpan={6} className="text-center py-8 text-muted-foreground">
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
