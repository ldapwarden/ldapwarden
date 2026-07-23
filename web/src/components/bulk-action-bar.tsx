import type { ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { X } from 'lucide-react'

/**
 * Toolbar shown above a list when a row selection is active. It owns the
 * "N selected" count and the clear button; the caller supplies the actual
 * action controls as children, so the same bar serves users and groups.
 */
export function BulkActionBar({
  count,
  onClear,
  children,
}: {
  count: number
  onClear: () => void
  children: ReactNode
}) {
  if (count === 0) return null
  return (
    <div className="flex flex-wrap items-center gap-2 rounded-md border bg-muted/50 px-3 py-2">
      <span className="text-sm font-medium">{count} selected</span>
      <div className="mx-1 h-4 w-px bg-border" aria-hidden="true" />
      <div className="flex flex-wrap items-center gap-2">{children}</div>
      <Button variant="ghost" size="sm" className="ml-auto" onClick={onClear}>
        <X className="h-4 w-4 mr-1" />
        Clear
      </Button>
    </div>
  )
}
