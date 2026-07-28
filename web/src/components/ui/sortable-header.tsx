import { ArrowDown, ArrowUp, ArrowUpDown } from 'lucide-react'
import { TableHead } from './table'
import { cn } from '@/lib/utils'
import type { SortDirection } from '@/lib/list-search'

interface SortableHeaderProps<S extends string> {
  field: S
  activeField: S
  direction: SortDirection
  onSort: (field: S) => void
  children: React.ReactNode
  className?: string
}

/**
 * A column header that sorts the table.
 *
 * Carries `aria-sort` on the `<th>` — without it a screen reader announces the
 * header as plain text and gives no indication the table is ordered, or by
 * which column. The button also states what the next click will do, since the
 * arrow glyph alone conveys that only visually.
 */
export function SortableHeader<S extends string>({
  field,
  activeField,
  direction,
  onSort,
  children,
  className,
}: SortableHeaderProps<S>) {
  const isActive = activeField === field
  const nextDirection = isActive && direction === 'asc' ? 'descending' : 'ascending'
  const Icon = !isActive ? ArrowUpDown : direction === 'asc' ? ArrowUp : ArrowDown

  return (
    <TableHead aria-sort={isActive ? (direction === 'asc' ? 'ascending' : 'descending') : 'none'} className={className}>
      <button
        type="button"
        className="flex items-center hover:text-foreground focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-ring rounded-sm"
        onClick={() => onSort(field)}
      >
        {children}
        <Icon
          className={cn('ml-1 h-4 w-4', isActive ? undefined : 'text-muted-foreground')}
          aria-hidden="true"
        />
        <span className="sr-only">, sort {nextDirection}</span>
      </button>
    </TableHead>
  )
}
