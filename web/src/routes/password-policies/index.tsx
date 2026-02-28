import { createFileRoute, Link, redirect } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Badge } from '@/components/ui/badge'
import { Pagination } from '@/components/ui/pagination'
import { Plus, Pencil, Trash2, Search, KeyRound } from 'lucide-react'
import { SortIcon } from '@/components/ui/sort-icon'
import { useState, useMemo } from 'react'
import { encodeDN } from '@/lib/utils'

type SortField = 'cn' | 'pwdMaxAge' | 'pwdMinLength'
type SortDirection = 'asc' | 'desc'

const DEFAULT_PAGE_SIZE = 25

export const Route = createFileRoute('/password-policies/')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
  },
  component: PasswordPoliciesPage,
})

function PasswordPoliciesPage() {
  const { hasPermission } = useAuth()
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [deletePolicy, setDeletePolicy] = useState<PasswordPolicy | null>(null)
  const [sortField, setSortField] = useState<SortField>('cn')
  const [sortDirection, setSortDirection] = useState<SortDirection>('asc')
  const [currentPage, setCurrentPage] = useState(1)
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE)

  const { data, isLoading, error } = useQuery({
    queryKey: ['password-policies'],
    queryFn: ({ signal }) => api.passwordPolicies.list(signal),
  })

  const deleteMutation = useMutation({
    mutationFn: (dn: string) => api.passwordPolicies.delete(dn),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['password-policies'] })
      setDeletePolicy(null)
    },
  })

  const canWrite = hasPermission('settings:write')

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc')
    } else {
      setSortField(field)
      setSortDirection('asc')
    }
  }

  const formatSeconds = (seconds: number | undefined) => {
    if (!seconds) return '-'
    if (seconds < 60) return `${seconds}s`
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m`
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`
    return `${Math.floor(seconds / 86400)}d`
  }

  const { sortedPolicies, totalFiltered, totalPages } = useMemo(() => {
    const filtered = data?.data.filter((policy) => {
      const searchLower = search.toLowerCase()
      return (
        policy.cn.toLowerCase().includes(searchLower) ||
        policy.description?.toLowerCase().includes(searchLower)
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
  }, [data?.data, search, sortField, sortDirection, currentPage, pageSize])

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
        Failed to load password policies: {error.message}
      </div>
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
            value={search}
            onChange={(e) => handleSearchChange(e.target.value)}
            className="pl-8"
          />
        </div>
        <span className="text-sm text-muted-foreground">
          {totalFiltered} policies
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
                    <SortIcon sortField={sortField} sortDirection={sortDirection} field="cn" />
                  </button>
                </TableHead>
                <TableHead>
                  <button
                    className="flex items-center hover:text-foreground"
                    onClick={() => handleSort('pwdMinLength')}
                  >
                    Min Length
                    <SortIcon sortField={sortField} sortDirection={sortDirection} field="pwdMinLength" />
                  </button>
                </TableHead>
                <TableHead>
                  <button
                    className="flex items-center hover:text-foreground"
                    onClick={() => handleSort('pwdMaxAge')}
                  >
                    Max Age
                    <SortIcon sortField={sortField} sortDirection={sortDirection} field="pwdMaxAge" />
                  </button>
                </TableHead>
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
                        <Button variant="ghost" size="icon">
                          <Pencil className="h-4 w-4" />
                        </Button>
                      </Link>
                      {canWrite && (
                        <Button
                          variant="ghost"
                          size="icon"
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
              onPageChange={setCurrentPage}
              onPageSizeChange={handlePageSizeChange}
            />
          )}
        </div>
      )}

      <Dialog open={!!deletePolicy} onOpenChange={() => setDeletePolicy(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Password Policy</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete policy "{deletePolicy?.cn}"?
              This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeletePolicy(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => deletePolicy && deleteMutation.mutate(deletePolicy.dn)}
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
