import { TableSkeletonRows } from '@/components/ui/skeleton'
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
import { TableEmptyState } from '@/components/ui/empty-state'
import { useCallback, useState, useRef } from 'react'
import { encodeDN } from '@/lib/utils'
import { useUrlSearchInput } from '@/lib/use-url-search-input'
import { useSearchHotkey } from '@/lib/use-hotkey'
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

  // `/` focuses the search box, the convention in keyboard-driven tools.
  const searchRef = useRef<HTMLInputElement>(null)
  useSearchHotkey(searchRef)

  // Search, sort and paging are applied by the directory side, so the total
  // describes the whole match rather than the slice in hand.
  const { data, isLoading, isFetching, error } = useQuery({
    queryKey: ['sudo-roles', { search: q, sortField, sortDirection, currentPage, pageSize }],
    queryFn: ({ signal }) =>
      api.sudoRoles.list(
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

  const sortedRoles = data?.data ?? []
  const totalFiltered = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(totalFiltered / pageSize))

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
      <div className="flex flex-wrap items-center justify-between gap-2">
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

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-48 flex-1 max-w-sm">
          <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
          <Input
            ref={searchRef}
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

      {/* Dim the table while a refetch is in flight: keepPreviousData leaves the
          old rows on screen, which would otherwise look like nothing happened
          after a sort or page change. */}
      <div className={`border rounded-lg ${isFetching && !isLoading ? 'opacity-60 transition-opacity' : ''}`}>
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
            {isLoading ? (
              <TableSkeletonRows rows={Math.min(pageSize, 8)} columns={6} />
            ) : (
              <>
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
              q ? (
                <TableEmptyState
                  colSpan={6}
                  icon={Search}
                  title={`No sudo roles match \u201c${q}\u201d`}
                  description="Search matches the role name and description."
                >
                  <Button variant="outline" size="sm" onClick={() => setSearchInput('')}>
                    Clear search
                  </Button>
                </TableEmptyState>
              ) : (
                <TableEmptyState
                  colSpan={6}
                  icon={ShieldCheck}
                  title="No sudo roles yet"
                  description={
                    canWrite
                      ? 'A sudo role grants commands to users or groups on given hosts.'
                      : 'The directory has no sudoRole entries under the configured OU.'
                  }
                >
                  {canWrite && (
                    <Link to="/sudo-roles/new">
                      <Button size="sm">
                        <Plus className="h-4 w-4 mr-1" />
                        New Sudo Role
                      </Button>
                    </Link>
                  )}
                </TableEmptyState>
              )
            )}
              </>
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
