import { TableSkeletonRows } from '@/components/ui/skeleton'
import { useRememberListSearch } from '@/lib/list-memory'
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
import { Checkbox } from '@/components/ui/checkbox'
import { BulkDeleteActions } from '@/components/bulk-delete-actions'
import { Badge } from '@/components/ui/badge'
import { Pagination } from '@/components/ui/pagination'
import { Plus, Trash2, Search, KeyRound } from 'lucide-react'
import { SortableHeader } from '@/components/ui/sortable-header'
import { Callout } from '@/components/ui/callout'
import { TableEmptyState } from '@/components/ui/empty-state'
import { useCallback, useState, useRef } from 'react'
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
  useRememberListSearch('/password-policies', searchParams)
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

  // `/` focuses the search box, the convention in keyboard-driven tools.
  const searchRef = useRef<HTMLInputElement>(null)
  useSearchHotkey(searchRef)

  // Search, sort and paging are applied by the directory side, so the total
  // describes the whole match rather than the slice in hand.
  const { data, isLoading, isFetching, error } = useQuery({
    queryKey: ['password-policies', { search: q, sortField, sortDirection, currentPage, pageSize }],
    queryFn: ({ signal }) =>
      api.passwordPolicies.list(
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

  const sortedPolicies = data?.data ?? []
  const totalFiltered = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(totalFiltered / pageSize))

  const {
    selected,
    selectedRows,
    allSelected,
    someSelected,
    toggleOne,
    toggleAll,
    clear: clearSelection,
  } = useRowSelection(sortedPolicies)

  const handleSort = (field: SortField) => {
    patchSearch(nextSort({ sort: sortField, dir: sortDirection }, field))
    clearSelection()
  }

  const formatSeconds = (seconds: number | undefined) => {
    if (!seconds) return '-'
    if (seconds < 60) return `${seconds}s`
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m`
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`
    return `${Math.floor(seconds / 86400)}d`
  }

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

  // The header stays when the list fails to load: the page is still the
  // page, and creating an entry does not depend on listing them.
  const header = (
    <div className="flex flex-wrap items-center justify-between gap-2">
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
  )

  if (error) {
    return (
      <div className="space-y-4">
        {header}
        <Callout variant="error">Failed to load password policies: {error.message}</Callout>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {header}

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-48 flex-1 max-w-sm">
          <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
          <Input
            ref={searchRef}
            placeholder="Search policies..."
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="pl-8"
          />
        </div>
        <span className="text-sm text-muted-foreground">
          {totalFiltered} polic{totalFiltered !== 1 ? 'ies' : 'y'}
        </span>
      </div>

      {data?.truncated && (
        <Callout variant="warning">
          Showing the first {data.total} matches. Refine your search to narrow the results.
        </Callout>
      )}

      {canWrite && (
        <BulkDeleteActions
          selected={selectedRows}
          onClear={clearSelection}
          noun="password policy"
          label={(policy) => policy.cn}
          remove={(policy) => api.passwordPolicies.delete(policy.dn)}
          invalidateKey={['password-policies']}
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
                    aria-label="Select all password policies"
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
              {canWrite && <TableHead className="w-16"><span className="sr-only">Actions</span></TableHead>}
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableSkeletonRows rows={Math.min(pageSize, 8)} columns={canWrite ? 7 : 5} />
            ) : (
              <>
                {sortedPolicies.map((policy) => (
              <TableRow key={policy.dn} data-state={selected.has(policy.dn) ? 'selected' : undefined}>
                {canWrite && (
                  <TableCell className="w-10">
                    <Checkbox
                      aria-label={`Select ${policy.cn}`}
                      checked={selected.has(policy.dn)}
                      onCheckedChange={() => toggleOne(policy.dn)}
                    />
                  </TableCell>
                )}
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
                    // Lockout being on is the protection working, not an alarm.
                    <Badge variant="outline">
                      after {policy.pwdMaxFailure || 0} attempts
                    </Badge>
                  ) : (
                    <Badge variant="secondary">Disabled</Badge>
                  )}
                </TableCell>
                <TableCell>
                  {policy.pwdInHistory ? `${policy.pwdInHistory} passwords` : '-'}
                </TableCell>
                {canWrite && (
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="Delete password policy"
                      onClick={() => setDeletePolicy(policy)}
                    >
                      <Trash2 className="h-4 w-4 text-destructive" />
                    </Button>
                  </TableCell>
                )}
              </TableRow>
            ))}
            {sortedPolicies.length === 0 && (
              q ? (
                <TableEmptyState
                  colSpan={canWrite ? 7 : 5}
                  icon={Search}
                  title={`No policies match \u201c${q}\u201d`}
                  description="Search matches the policy name and description."
                >
                  <Button variant="outline" size="sm" onClick={() => setSearchInput('')}>
                    Clear search
                  </Button>
                </TableEmptyState>
              ) : (
                <TableEmptyState
                  colSpan={canWrite ? 7 : 5}
                  icon={KeyRound}
                  title="No password policies yet"
                  description={
                    canWrite
                      ? 'A policy sets password length, history and lockout rules, and can then be assigned to users.'
                      : 'The directory has no pwdPolicy entries under the configured OU.'
                  }
                >
                  {canWrite && (
                    <Link to="/password-policies/new">
                      <Button size="sm">
                        <Plus className="h-4 w-4 mr-1" />
                        New Policy
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
