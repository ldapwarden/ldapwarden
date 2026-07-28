import { InlineSpinner } from '@/components/inline-spinner'
import { createFileRoute, Link, redirect } from '@tanstack/react-router'
import { useQuery, keepPreviousData, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { runBulk, bulkSummary } from '@/lib/bulk'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { BulkActionBar } from '@/components/bulk-action-bar'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Pagination } from '@/components/ui/pagination'
import { Plus, Search, Users, Trash2, Upload } from 'lucide-react'
import { SortIcon } from '@/components/ui/sort-icon'
import { Callout } from '@/components/ui/callout'
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
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [sortField, setSortField] = useState<SortField>('cn')
  const [sortDirection, setSortDirection] = useState<SortDirection>('asc')
  const [currentPage, setCurrentPage] = useState(1)
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const debouncedSearch = useDebounced(search)

  // Search, sort and paging are applied by the directory side; the browser
  // renders one page of a light projection.
  const { data, isLoading, isFetching, error } = useQuery({
    queryKey: ['groups', { search: debouncedSearch, sortField, sortDirection, currentPage, pageSize }],
    queryFn: ({ signal }) =>
      api.groups.list(
        {
          search: debouncedSearch,
          sort: sortField,
          order: sortDirection,
          limit: pageSize,
          offset: (currentPage - 1) * pageSize,
        },
        signal,
      ),
    placeholderData: keepPreviousData,
  })

  // Memoised so the derived selection below keeps a stable identity between
  // renders.
  const groups = useMemo(() => data?.data ?? [], [data?.data])
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  const canWrite = hasPermission('groups:write')

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc')
    } else {
      setSortField(field)
      setSortDirection('asc')
    }
    setCurrentPage(1)
  }

  // Selection only spans the visible page: with paging done server-side the
  // rest of the result set is not in memory, and a bulk delete must never
  // reach rows the operator has not seen.
  const selectedGroups = useMemo(() => groups.filter((g) => selected.has(g.dn)), [groups, selected])
  const allSelected = groups.length > 0 && groups.every((g) => selected.has(g.dn))
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
    setSelected((prev) => (groups.every((g) => prev.has(g.dn)) ? new Set() : new Set(groups.map((g) => g.dn))))
  }
  const clearSelection = () => setSelected(new Set())

  const bulkDelete = async () => {
    setDeleting(true)
    try {
      const result = await runBulk(selectedGroups, (g) => api.groups.delete(g.dn))
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      const summary = bulkSummary('Deleted', 'group', result)
      if (result.failed.length === 0) toast.success(summary)
      else toast.error(`${summary} First error: ${result.failed[0].error}`)
      clearSelection()
      setDeleteOpen(false)
    } finally {
      setDeleting(false)
    }
  }

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
    return (
      <Callout variant="error">
        Failed to load groups: {error.message}
      </Callout>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Groups</h1>
        {canWrite && (
          <div className="flex items-center gap-2">
            <Link to="/groups/import">
              <Button variant="outline">
                <Upload className="h-4 w-4 mr-1" />
                Import CSV
              </Button>
            </Link>
            <Link to="/groups/new">
              <Button>
                <Plus className="h-4 w-4 mr-1" />
                New Group
              </Button>
            </Link>
          </div>
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
          {total} group{total !== 1 ? 's' : ''}
        </span>
      </div>

      {data?.truncated && (
        <Callout variant="warning">
          Showing the first {data.total} matches. Refine your search to narrow the results.
        </Callout>
      )}

      {canWrite && (
        <BulkActionBar count={selectedGroups.length} onClear={clearSelection}>
          <Button variant="destructive" size="sm" disabled={deleting} onClick={() => setDeleteOpen(true)}>
            <Trash2 className="h-4 w-4 mr-1" />
            Delete
          </Button>
        </BulkActionBar>
      )}

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={(open) => { if (!open) setDeleteOpen(false) }}
        title="Delete groups"
        description={`Delete ${selectedGroups.length} selected group${selectedGroups.length === 1 ? '' : 's'}? This permanently removes them from the directory and cannot be undone.`}
        confirmLabel={`Delete ${selectedGroups.length}`}
        pendingLabel="Deleting..."
        isPending={deleting}
        onConfirm={bulkDelete}
      />

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
                      aria-label="Select all groups"
                      checked={allSelected}
                      indeterminate={someSelected}
                      onCheckedChange={toggleAll}
                    />
                  </TableHead>
                )}
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
              {groups.map((group) => (
                <TableRow key={group.dn} data-state={selected.has(group.dn) ? 'selected' : undefined}>
                  {canWrite && (
                    <TableCell className="w-10">
                      <Checkbox
                        aria-label={`Select ${group.cn}`}
                        checked={selected.has(group.dn)}
                        onCheckedChange={() => toggleOne(group.dn)}
                      />
                    </TableCell>
                  )}
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
                      {group.membersCount ?? 0}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {groups.length === 0 && (
                <TableRow>
                  <TableCell colSpan={canWrite ? 5 : 4} className="text-center py-8 text-muted-foreground">
                    No groups found
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
