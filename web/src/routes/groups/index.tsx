import { createFileRoute, Link, redirect } from '@tanstack/react-router'
import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Pagination } from '@/components/ui/pagination'
import { Plus, Search, Users } from 'lucide-react'
import { SortIcon } from '@/components/ui/sort-icon'
import { useState, useMemo } from 'react'
import { encodeDN } from '@/lib/utils'
import { useDebounced } from '@/lib/use-debounced'

type SortField = 'cn' | 'description' | 'gidNumber' | 'members'
type SortDirection = 'asc' | 'desc'

const DEFAULT_PAGE_SIZE = 25

export const Route = createFileRoute('/groups/')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
  },
  component: GroupsPage,
})

function GroupsPage() {
  const { hasPermission } = useAuth()
  const [search, setSearch] = useState('')
  const [sortField, setSortField] = useState<SortField>('cn')
  const [sortDirection, setSortDirection] = useState<SortDirection>('asc')
  const [currentPage, setCurrentPage] = useState(1)
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE)
  const debouncedSearch = useDebounced(search)

  const { data, isLoading, error } = useQuery({
    queryKey: ['groups', debouncedSearch],
    queryFn: ({ signal }) => api.groups.list(debouncedSearch, signal),
    placeholderData: keepPreviousData,
  })

  const canWrite = hasPermission('groups:write')

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc')
    } else {
      setSortField(field)
      setSortDirection('asc')
    }
  }

  const { sortedGroups, totalFiltered, totalPages } = useMemo(() => {
    // Search is applied server-side; sort and paginate the returned set here.
    const sorted = [...(data?.data ?? [])].sort((a, b) => {
      let aVal: string | number = ''
      let bVal: string | number = ''

      switch (sortField) {
        case 'cn':
          aVal = a.cn.toLowerCase()
          bVal = b.cn.toLowerCase()
          break
        case 'description':
          aVal = (a.description || '').toLowerCase()
          bVal = (b.description || '').toLowerCase()
          break
        case 'gidNumber':
          aVal = a.gidNumber
          bVal = b.gidNumber
          break
        case 'members':
          aVal = a.memberUid?.length ?? 0
          bVal = b.memberUid?.length ?? 0
          break
      }

      if (aVal < bVal) return sortDirection === 'asc' ? -1 : 1
      if (aVal > bVal) return sortDirection === 'asc' ? 1 : -1
      return 0
    })

    const totalPages = Math.ceil(sorted.length / pageSize)
    const startIndex = (currentPage - 1) * pageSize
    const paginatedGroups = sorted.slice(startIndex, startIndex + pageSize)

    return {
      sortedGroups: paginatedGroups,
      totalFiltered: sorted.length,
      totalPages,
    }
  }, [data?.data, sortField, sortDirection, currentPage, pageSize])

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
        Failed to load groups: {error.message}
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Groups</h1>
        {canWrite && (
          <Link to="/groups/new">
            <Button>
              <Plus className="h-4 w-4 mr-1" />
              New Group
            </Button>
          </Link>
        )}
      </div>

      <div className="flex items-center gap-2">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
          <Input
            placeholder="Search groups..."
            value={search}
            onChange={(e) => handleSearchChange(e.target.value)}
            className="pl-8"
          />
        </div>
        <span className="text-sm text-muted-foreground">
          {totalFiltered} groups
        </span>
      </div>

      {data?.truncated && (
        <div className="rounded-md border border-yellow-500/40 bg-yellow-500/10 px-3 py-2 text-sm text-yellow-700 dark:text-yellow-500">
          Showing the first {data.total} matches. Refine your search to narrow the results.
        </div>
      )}

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
                    onClick={() => handleSort('cn')}
                  >
                    Name
                    <SortIcon sortField={sortField} sortDirection={sortDirection} field="cn" />
                  </button>
                </TableHead>
                <TableHead>
                  <button
                    className="flex items-center hover:text-foreground"
                    onClick={() => handleSort('description')}
                  >
                    Description
                    <SortIcon sortField={sortField} sortDirection={sortDirection} field="description" />
                  </button>
                </TableHead>
                <TableHead>
                  <button
                    className="flex items-center hover:text-foreground"
                    onClick={() => handleSort('gidNumber')}
                  >
                    GID
                    <SortIcon sortField={sortField} sortDirection={sortDirection} field="gidNumber" />
                  </button>
                </TableHead>
                <TableHead>
                  <button
                    className="flex items-center hover:text-foreground"
                    onClick={() => handleSort('members')}
                  >
                    Members
                    <SortIcon sortField={sortField} sortDirection={sortDirection} field="members" />
                  </button>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sortedGroups.map((group) => (
                <TableRow key={group.dn}>
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
                      {group.memberUid?.length ?? 0}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {sortedGroups.length === 0 && (
                <TableRow>
                  <TableCell colSpan={4} className="text-center py-8 text-muted-foreground">
                    No groups found
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
