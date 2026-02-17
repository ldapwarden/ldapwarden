import { createFileRoute, Link, redirect } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type SudoRole } from '@/lib/api'
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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Pagination } from '@/components/ui/pagination'
import { Plus, Pencil, Trash2, Search, ArrowUp, ArrowDown, ArrowUpDown, ShieldCheck } from 'lucide-react'
import { useState, useMemo } from 'react'
import { encodeDN } from '@/lib/utils'

type SortField = 'cn' | 'sudoOrder'
type SortDirection = 'asc' | 'desc'

const DEFAULT_PAGE_SIZE = 25

export const Route = createFileRoute('/sudo-roles/')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
  },
  component: SudoRolesPage,
})

function SudoRolesPage() {
  const { hasPermission } = useAuth()
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [deleteRole, setDeleteRole] = useState<SudoRole | null>(null)
  const [sortField, setSortField] = useState<SortField>('cn')
  const [sortDirection, setSortDirection] = useState<SortDirection>('asc')
  const [currentPage, setCurrentPage] = useState(1)
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE)

  const { data, isLoading, error } = useQuery({
    queryKey: ['sudo-roles'],
    queryFn: ({ signal }) => api.sudoRoles.list(signal),
  })

  const deleteMutation = useMutation({
    mutationFn: (dn: string) => api.sudoRoles.delete(dn),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['sudo-roles'] })
      setDeleteRole(null)
    },
  })

  const canWrite = hasPermission('users:write')
  const canDelete = hasPermission('users:delete')

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

  const { sortedRoles, totalFiltered, totalPages } = useMemo(() => {
    const filtered = data?.data.filter((role) => {
      const searchLower = search.toLowerCase()
      return (
        role.cn.toLowerCase().includes(searchLower) ||
        role.description?.toLowerCase().includes(searchLower) ||
        role.sudoCommand?.some(cmd => cmd.toLowerCase().includes(searchLower)) ||
        role.sudoUser?.some(user => user.toLowerCase().includes(searchLower))
      )
    }) ?? []

    const sorted = [...filtered].sort((a, b) => {
      let aVal: string | number = ''
      let bVal: string | number = ''

      switch (sortField) {
        case 'cn':
          aVal = a.cn.toLowerCase()
          bVal = b.cn.toLowerCase()
          break
        case 'sudoOrder':
          aVal = a.sudoOrder ?? 0
          bVal = b.sudoOrder ?? 0
          break
      }

      if (aVal < bVal) return sortDirection === 'asc' ? -1 : 1
      if (aVal > bVal) return sortDirection === 'asc' ? 1 : -1
      return 0
    })

    const totalPages = Math.ceil(sorted.length / pageSize)
    const startIndex = (currentPage - 1) * pageSize
    const paginatedRoles = sorted.slice(startIndex, startIndex + pageSize)

    return {
      sortedRoles: paginatedRoles,
      totalFiltered: sorted.length,
      totalPages,
    }
  }, [data?.data, search, sortField, sortDirection, currentPage, pageSize])

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
        Failed to load sudo roles: {error.message}
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Sudo Roles</h1>
        {canWrite && (
          <Link to="/sudo-roles/new">
            <Button>
              <Plus className="h-4 w-4 mr-1" />
              New Sudo Role
            </Button>
          </Link>
        )}
      </div>

      <div className="flex items-center gap-2">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
          <Input
            placeholder="Search sudo roles..."
            value={search}
            onChange={(e) => handleSearchChange(e.target.value)}
            className="pl-8"
          />
        </div>
        <span className="text-sm text-muted-foreground">
          {totalFiltered} sudo roles
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
                    onClick={() => handleSort('cn')}
                  >
                    Name
                    <SortIcon field="cn" />
                  </button>
                </TableHead>
                <TableHead>Users</TableHead>
                <TableHead>Hosts</TableHead>
                <TableHead>Commands</TableHead>
                <TableHead>
                  <button
                    className="flex items-center hover:text-foreground"
                    onClick={() => handleSort('sudoOrder')}
                  >
                    Order
                    <SortIcon field="sudoOrder" />
                  </button>
                </TableHead>
                <TableHead className="w-[100px]">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sortedRoles.map((role) => (
                <TableRow key={role.dn}>
                  <TableCell>
                    <Link
                      to="/sudo-roles/$dn"
                      params={{ dn: encodeDN(role.dn) }}
                      className="flex items-center gap-2 hover:underline"
                    >
                      <ShieldCheck className="h-4 w-4 text-muted-foreground" />
                      <span className="font-medium">{role.cn}</span>
                    </Link>
                  </TableCell>
                  <TableCell>
                    <span className="text-sm text-muted-foreground">
                      {role.sudoUser?.length ? role.sudoUser.slice(0, 3).join(', ') : '-'}
                      {role.sudoUser && role.sudoUser.length > 3 && ` +${role.sudoUser.length - 3}`}
                    </span>
                  </TableCell>
                  <TableCell>
                    <span className="text-sm text-muted-foreground">
                      {role.sudoHost?.length ? role.sudoHost.slice(0, 2).join(', ') : '-'}
                      {role.sudoHost && role.sudoHost.length > 2 && ` +${role.sudoHost.length - 2}`}
                    </span>
                  </TableCell>
                  <TableCell>
                    <span className="text-sm text-muted-foreground font-mono">
                      {role.sudoCommand?.length ? role.sudoCommand.slice(0, 2).join(', ') : '-'}
                      {role.sudoCommand && role.sudoCommand.length > 2 && ` +${role.sudoCommand.length - 2}`}
                    </span>
                  </TableCell>
                  <TableCell>{role.sudoOrder || '-'}</TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1">
                      <Link to="/sudo-roles/$dn" params={{ dn: encodeDN(role.dn) }}>
                        <Button variant="ghost" size="icon">
                          <Pencil className="h-4 w-4" />
                        </Button>
                      </Link>
                      {canDelete && (
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => setDeleteRole(role)}
                        >
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {sortedRoles.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6} className="text-center py-8 text-muted-foreground">
                    No sudo roles found
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

      <Dialog open={!!deleteRole} onOpenChange={() => setDeleteRole(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Sudo Role</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete sudo role "{deleteRole?.cn}"?
              This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteRole(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => deleteRole && deleteMutation.mutate(deleteRole.dn)}
              disabled={deleteMutation.isPending}
            >
              {deleteMutation.isPending ? 'Deleting...' : 'Delete'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
