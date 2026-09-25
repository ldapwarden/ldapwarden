import { cn } from '@/lib/utils'
import { TableCell, TableRow } from './table'

/**
 * A shimmering placeholder block. Sized by the caller so it occupies the same
 * space the real content will, which is the point: a spinner tells you to wait
 * and then the page jumps, a skeleton shows where things will land.
 */
export function Skeleton({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn('animate-pulse rounded-md bg-muted', className)}
      aria-hidden="true"
      {...props}
    />
  )
}

/**
 * Placeholder rows for a table that is still loading. Rendered inside the real
 * table so the header, toolbar and border stay put; only the body fills in.
 *
 * The whole block is announced once via a `status` role on the first row rather
 * than per cell, so a screen reader hears "Loading…" instead of a wall of
 * nothing.
 */
export function TableSkeletonRows({
  rows = 8,
  columns,
  label = 'Loading…',
  hideBelowMd = [],
}: {
  rows?: number
  columns: number
  label?: string
  /** Column indexes the table hides below md, so the placeholder matches. */
  hideBelowMd?: number[]
}) {
  return (
    <>
      {Array.from({ length: rows }, (_, row) => (
        <TableRow key={row}>
          {Array.from({ length: columns }, (_, column) => (
            <TableCell key={column} className={hideBelowMd.includes(column) ? 'hidden md:table-cell' : undefined}>
              {row === 0 && column === 0 && <span className="sr-only" role="status">{label}</span>}
              <Skeleton
                className="h-4"
                // Vary the widths a little so the block reads as text rather
                // than as a grid of identical bars.
                style={{ width: `${[70, 55, 85, 45, 60][(row + column) % 5]}%` }}
              />
            </TableCell>
          ))}
        </TableRow>
      ))}
    </>
  )
}

/** Placeholder for a detail page: title block plus a couple of cards. */
export function DetailSkeleton({ cards = 2 }: { cards?: number }) {
  return (
    <div className="space-y-4">
      <span className="sr-only" role="status">Loading…</span>
      <div className="flex items-center gap-4">
        <Skeleton className="h-16 w-16 rounded-full" />
        <div className="flex-1 space-y-2">
          <Skeleton className="h-6 w-64" />
          <Skeleton className="h-4 w-96 max-w-full" />
        </div>
      </div>
      <Skeleton className="h-10 w-80 max-w-full" />
      <div className="grid gap-4 md:grid-cols-2">
        {Array.from({ length: cards }, (_, i) => (
          <div key={i} className="space-y-3 rounded-lg border p-6">
            <Skeleton className="h-5 w-40" />
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-9 w-full" />
            <Skeleton className="h-9 w-full" />
            <Skeleton className="h-9 w-2/3" />
          </div>
        ))}
      </div>
    </div>
  )
}
