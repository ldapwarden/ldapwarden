import { InlineSpinner } from '@/components/inline-spinner'
import { createFileRoute, Link, redirect } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import { api, type PasswordPolicy } from '@/lib/api'
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
import { Badge } from '@/components/ui/badge'
import { Pagination } from '@/components/ui/pagination'
import { Plus, Pencil, Trash2, Search, KeyRound } from 'lucide-react'
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

const SORT_FIELDS = ['cn', 'pwdMaxAge', 'pwdMinLength'] as const
type SortField = (typeof SORT_FIELDS)[number]

export const Route = createFileRoute('/password-policies/')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
  },
  validateSearch: listSearchValidator(SORT_FIELDS),
  component: PasswordPoliciesPage,
})

function PasswordPoliciesPage() {
  const { hasPermission } = useAuth()
  const queryClient = useQueryClient()
  const searchParams = Route.useSearch()
  const navigate = Route.useNavigate()
  const { q, sort: sortField, dir: sortDirection, page: currentPage, size: pageSize } =
    listSearchValues<SortField>(searchParams, 'cn')
  const [deletePolicy, setDeletePolicy] = useState<PasswordPolicy | null>(null)

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
    queryKey: ['password-policies', q],
    queryFn: ({ signal }) => api.passwordPolicies.list(q, signal),
    placeholderData: keepPreviousData,
  })

  const deleteMutation = useMutation({
    mutationFn: (dn: string) => api.passwordPolicies.delete(dn),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['password-policies'] })
      setDeletePolicy(null)
      toast.success('Password policy deleted')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const canWrite = hasPermission('settings:write')

  const handleSort = (field: SortField) => {
    patchSearch(nextSort({ sort: sortField, dir: sortDirection }, field))
  }

  const formatSeconds = (seconds: number | undefined) => {
    if (!seconds) return '-'
    if (seconds < 60) return `${seconds}s`
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m`
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`
    return `${Math.floor(seconds / 86400)}d`
  }

  const { sortedPolicies, totalFiltered, totalPages } = useMemo(() => {
    // Search is applied server-side; sort and paginate the returned set here.
    const sorted = [...(data?.data ?? [])].sort((a, b) => {
      let aVal: string | number = ''
      let bVal: string | number = ''

      switch (sortField) {
        case 'cn':
          aVal = a.cn.toLowerCase()
          bVal = b.cn.toLowerCase()
          break
        case 'pwdMaxAge':
          aVal = a.pwdMaxAge ?? 0
          bVal = b.pwdMaxAge ?? 0
          break
        case 'pwdMinLength':
          aVal = a.pwdMinLength ?? 0
          bVal = b.pwdMinLength ?? 0
          break
      }

      if (aVal < bVal) return sortDirection === 'asc' ? -1 : 1
      if (aVal > bVal) return sortDirection === 'asc' ? 1 : -1
      return 0
    })

    const totalPages = Math.ceil(sorted.length / pageSize)
    const startIndex = (currentPage - 1) * pageSize
    const paginatedPolicies = sorted.slice(startIndex, startIndex + pageSize)

    return {
      sortedPolicies: paginatedPolicies,
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
        Failed to load password policies: {error.message}
      </Callout>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Password Policies</h1>
        {canWrite && (
          <Link to="/password-policies/new">
            <Button>
              <Plus className="h-4 w-4 mr-1" />
              New Policy
            </Button>
          </Link>
        )}
      </div>

      <div className="flex items-center gap-2">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
          <Input
            placeholder="Search policies..."
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="pl-8"
          />
        </div>
        <span className="text-sm text-muted-foreground">
          {totalFiltered} policies
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
                <SortableHeader
                  field="pwdMinLength"
                  activeField={sortField}
                  direction={sortDirection}
                  onSort={handleSort}
                >
                  Min Length
                </SortableHeader>
                <SortableHeader
                  field="pwdMaxAge"
                  activeField={sortField}
                  direction={sortDirection}
                  onSort={handleSort}
                >
                  Max Age
                </SortableHeader>
                <TableHead>Lockout</TableHead>
                <TableHead>History</TableHead>
                <TableHead className="w-[100px]">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sortedPolicies.map((policy) => (
                <TableRow key={policy.dn}>
                  <TableCell>
                    <Link
                      to="/password-policies/$dn"
                      params={{ dn: encodeDN(policy.dn) }}
                      className="flex items-center gap-2 hover:underline"
                    >
                      <KeyRound className="h-4 w-4 text-muted-foreground" />
                      <div>
                        <span className="font-medium">{policy.cn}</span>
                        {policy.description && (
                          <p className="text-xs text-muted-foreground">{policy.description}</p>
                        )}
                      </div>
                    </Link>
                  </TableCell>
                  <TableCell>
                    {policy.pwdMinLength ? `${policy.pwdMinLength} chars` : '-'}
                  </TableCell>
                  <TableCell>
                    {formatSeconds(policy.pwdMaxAge)}
                  </TableCell>
                  <TableCell>
                    {policy.pwdLockout ? (
                      <Badge variant="destructive">
                        {policy.pwdMaxFailure || 0} attempts
                      </Badge>
                    ) : (
                      <Badge variant="secondary">Disabled</Badge>
                    )}
                  </TableCell>
                  <TableCell>
                    {policy.pwdInHistory ? `${policy.pwdInHistory} passwords` : '-'}
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1">
                      <Link to="/password-policies/$dn" params={{ dn: encodeDN(policy.dn) }}>
                        <Button variant="ghost" size="icon" aria-label="Edit password policy">
                          <Pencil className="h-4 w-4" />
                        </Button>
                      </Link>
                      {canWrite && (
                        <Button
                          variant="ghost"
                          size="icon"
                          aria-label="Delete password policy"
                          onClick={() => setDeletePolicy(policy)}
                        >
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {sortedPolicies.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6} className="text-center py-8 text-muted-foreground">
                    No password policies found
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
        open={!!deletePolicy}
        onOpenChange={(open) => !open && setDeletePolicy(null)}
        title="Delete Password Policy"
        description={`Are you sure you want to delete policy "${deletePolicy?.cn}"? This action cannot be undone.`}
        error={deleteMutation.error?.message}
        isPending={deleteMutation.isPending}
        onConfirm={() => deletePolicy && deleteMutation.mutate(deletePolicy.dn)}
      />
    </div>
  )
}
