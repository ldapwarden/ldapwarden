import type { ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { X } from 'lucide-react'

/**
 * Toolbar shown above a list when a row selection is active. It owns the
 * "N selected" count, the run progress and the clear button; the caller
 * supplies the actual action controls as children, so the same bar serves users
 * and groups.
 *
 * Sticky, because it was previously rendered inline above the table: on a
 * hundred-row page the operator scrolls down to pick rows and the actions were
 * left off-screen behind them.
 */
export function BulkActionBar({
  count,
  onClear,
  progress,
  children,
}: {
  count: number
  onClear: () => void
  /** Set while a bulk action is running, so the wait is visible. */
  progress?: { done: number; total: number } | null
  children: ReactNode
}) {
  if (count === 0) return null
  const running = progress != null && progress.done < progress.total
  return (
    <div className="sticky top-0 z-10 flex flex-wrap items-center gap-2 rounded-md border bg-muted px-3 py-2 shadow-xs">
      <span className="text-sm font-medium">{count} selected</span>
      <div className="mx-1 h-4 w-px bg-border" aria-hidden="true" />
      <div className="flex flex-wrap items-center gap-2">{children}</div>
      {progress && (
        <span
          className="text-sm text-muted-foreground tabular-nums"
          role="status"
          aria-live="polite"
        >
          {running ? `${progress.done} of ${progress.total} done…` : 'Finishing…'}
        </span>
      )}
      <Button variant="ghost" size="sm" className="ml-auto" onClick={onClear} disabled={running}>
        <X className="h-4 w-4 mr-1" />
        Clear
      </Button>
    </div>
  )
}
