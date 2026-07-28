import { InlineSpinner } from '@/components/inline-spinner'
import { createFileRoute, Link, redirect } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
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
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Pagination } from '@/components/ui/pagination'
import { Plus, Pencil, Trash2, Search, ShieldCheck } from 'lucide-react'
import { SortableHeader } from '@/components/ui/sortable-header'
import { Callout } from '@/components/ui/callout'
import { useCallback, useMemo, useState } from 'react'
import { encodeDN } from '@/lib/utils'
import { useUrlSearchInput } from '@/lib/use-url-search-input'
import {
  listSearchValidator,
  listSearchValues,
  nextSort,
  type ListSearch,
} from '@/lib/list-search'
import { toast } from 'sonner'

const SORT_FIELDS = ['cn', 'sudoOrder'] as const
type SortField = (typeof SORT_FIELDS)[number]

export const Route = createFileRoute('/sudo-roles/')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
  },
  validateSearch: listSearchValidator(SORT_FIELDS),
  component: SudoRolesPage,
})

function SudoRolesPage() {
  const { hasPermission } = useAuth()
  const queryClient = useQueryClient()
  const searchParams = Route.useSearch()
  const navigate = Route.useNavigate()
  const { q, sort: sortField, dir: sortDirection, page: currentPage, size: pageSize } =
    listSearchValues<SortField>(searchParams, 'cn')
  const [deleteRole, setDeleteRole] = useState<SudoRole | null>(null)

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

  const { data, isLoading, error } = useQuery({
    queryKey: ['sudo-roles', q],
    queryFn: ({ signal }) => api.sudoRoles.list(q, signal),
    placeholderData: keepPreviousData,
  })

  const deleteMutation = useMutation({
    mutationFn: (dn: string) => api.sudoRoles.delete(dn),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['sudo-roles'] })
      setDeleteRole(null)
      toast.success('Sudo role deleted')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const canWrite = hasPermission('users:write')
  const canDelete = hasPermission('users:delete')

  const handleSort = (field: SortField) => {
    patchSearch(nextSort({ sort: sortField, dir: sortDirection }, field))
  }

  const { sortedRoles, totalFiltered, totalPages } = useMemo(() => {
    // Search is applied server-side; sort and paginate the returned set here.
    const sorted = [...(data?.data ?? [])].sort((a, b) => {
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
  }, [data?.data, sortField, sortDirection, currentPage, pageSize])

  const handlePageChange = (page: number) => {
    // Paging is the one move worth a history entry, so Back returns to the
    // previous page instead of leaving the list entirely.
    patchSearch({ page: page > 1 ? page : undefined }, false)
  }

  const handlePageSizeChange = (size: number) => {
    patchSearch({ size, page: undefined })
  }

  if (error) {
    return (
      <Callout variant="error">
        Failed to load sudo roles: {error.message}
      </Callout>
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
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="pl-8"
          />
        </div>
        <span className="text-sm text-muted-foreground">
          {totalFiltered} sudo roles
        </span>
      </div>

      {data?.truncated && (
        <Callout variant="warning">
          Showing the first {data.total} matches. Refine your search to narrow the results.
        </Callout>
      )}

      {isLoading ? (
        <InlineSpinner />
      ) : (
        <div className="border rounded-lg">
          <Table>
            <TableHeader>
              <TableRow>
                <SortableHeader
                  field="cn"
                  activeField={sortField}
                  direction={sortDirection}
                  onSort={handleSort}
                >
                  Name
                </SortableHeader>
                <TableHead>Users</TableHead>
                <TableHead>Hosts</TableHead>
                <TableHead>Commands</TableHead>
                <SortableHeader
                  field="sudoOrder"
                  activeField={sortField}
                  direction={sortDirection}
                  onSort={handleSort}
                >
                  Order
                </SortableHeader>
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
                    <span className="text-sm text-muted-foreground" title={role.sudoUser?.join(', ') || undefined}>
                      {role.sudoUser?.length ? role.sudoUser.slice(0, 3).join(', ') : '-'}
                      {role.sudoUser && role.sudoUser.length > 3 && ` +${role.sudoUser.length - 3}`}
                    </span>
                  </TableCell>
                  <TableCell>
                    <span className="text-sm text-muted-foreground" title={role.sudoHost?.join(', ') || undefined}>
                      {role.sudoHost?.length ? role.sudoHost.slice(0, 2).join(', ') : '-'}
                      {role.sudoHost && role.sudoHost.length > 2 && ` +${role.sudoHost.length - 2}`}
                    </span>
                  </TableCell>
                  <TableCell>
                    <span className="text-sm text-muted-foreground font-mono" title={role.sudoCommand?.join(', ') || undefined}>
                      {role.sudoCommand?.length ? role.sudoCommand.slice(0, 2).join(', ') : '-'}
                      {role.sudoCommand && role.sudoCommand.length > 2 && ` +${role.sudoCommand.length - 2}`}
                    </span>
                  </TableCell>
                  <TableCell>{role.sudoOrder || '-'}</TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1">
                      <Link to="/sudo-roles/$dn" params={{ dn: encodeDN(role.dn) }}>
                        <Button variant="ghost" size="icon" aria-label="Edit sudo role">
                          <Pencil className="h-4 w-4" />
                        </Button>
                      </Link>
                      {canDelete && (
                        <Button
                          variant="ghost"
                          size="icon"
                          aria-label="Delete sudo role"
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
              onPageChange={handlePageChange}
              onPageSizeChange={handlePageSizeChange}
            />
          )}
        </div>
      )}

      <ConfirmDialog
        open={!!deleteRole}
        onOpenChange={(open) => !open && setDeleteRole(null)}
        title="Delete Sudo Role"
        description={`Are you sure you want to delete sudo role "${deleteRole?.cn}"? This action cannot be undone.`}
        error={deleteMutation.error?.message}
        isPending={deleteMutation.isPending}
        onConfirm={() => deleteRole && deleteMutation.mutate(deleteRole.dn)}
      />
    </div>
  )
}
