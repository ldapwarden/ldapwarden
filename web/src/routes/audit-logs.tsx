import { createFileRoute, redirect } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Pagination } from '@/components/ui/pagination'
import { useState } from 'react'

const DEFAULT_PAGE_SIZE = 25

export const Route = createFileRoute('/audit-logs')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
    if (!context.auth.hasPermission('audit:read')) {
      throw redirect({ to: '/' })
    }
  },
  component: AuditLogsPage,
})

function AuditLogsPage() {
  const [currentPage, setCurrentPage] = useState(1)
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE)

  const { data, isLoading, error } = useQuery({
    queryKey: ['audit-logs', currentPage, pageSize],
    queryFn: () => api.auditLogs.list({ limit: pageSize, offset: (currentPage - 1) * pageSize }),
  })

  const totalPages = data ? Math.ceil(data.total / pageSize) : 0

  const handlePageSizeChange = (size: number) => {
    setPageSize(size)
    setCurrentPage(1)
  }

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleString()
  }

  const getActionBadgeClass = (action: string) => {
    if (action.includes('create')) return 'bg-green-100 text-green-800'
    if (action.includes('delete')) return 'bg-red-100 text-red-800'
    if (action.includes('update')) return 'bg-blue-100 text-blue-800'
    if (action.includes('login')) return 'bg-purple-100 text-purple-800'
    if (action.includes('logout')) return 'bg-gray-100 text-gray-800'
    return 'bg-gray-100 text-gray-800'
  }

  if (error) {
    return (
      <div className="p-4 text-destructive bg-destructive/10 rounded-md">
        Failed to load audit logs: {error.message}
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Audit Logs</h1>
        <span className="text-sm text-muted-foreground">
          {data?.total ?? 0} total entries
        </span>
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-8">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
        </div>
      ) : (
        <>
          <div className="border rounded-lg">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Timestamp</TableHead>
                  <TableHead>Actor</TableHead>
                  <TableHead>Action</TableHead>
                  <TableHead>Resource</TableHead>
                  <TableHead>Details</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data?.data.map((log) => (
                  <TableRow key={log.id}>
                    <TableCell className="whitespace-nowrap">
                      {formatDate(log.createdAt)}
                    </TableCell>
                    <TableCell>
                      <span className="font-medium">{log.actorUid}</span>
                    </TableCell>
                    <TableCell>
                      <span
                        className={`px-2 py-1 rounded-full text-xs font-medium ${getActionBadgeClass(
                          log.action
                        )}`}
                      >
                        {log.action}
                      </span>
                    </TableCell>
                    <TableCell>
                      <div className="max-w-xs truncate">
                        <span className="text-muted-foreground">{log.resourceType}: </span>
                        {log.resourceDn || '-'}
                      </div>
                    </TableCell>
                    <TableCell>
                      {log.details && Object.keys(log.details).length > 0 ? (
                        <code className="text-xs bg-muted px-1 py-0.5 rounded">
                          {JSON.stringify(log.details)}
                        </code>
                      ) : (
                        '-'
                      )}
                    </TableCell>
                  </TableRow>
                ))}
                {data?.data.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={5} className="text-center py-8 text-muted-foreground">
                      No audit logs found
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
            {data && data.total > 0 && (
              <Pagination
                currentPage={currentPage}
                totalPages={totalPages}
                pageSize={pageSize}
                totalItems={data.total}
                onPageChange={setCurrentPage}
                onPageSizeChange={handlePageSizeChange}
              />
            )}
          </div>
        </>
      )}
    </div>
  )
}
