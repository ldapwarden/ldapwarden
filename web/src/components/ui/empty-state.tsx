import type { LucideIcon } from 'lucide-react'
import { TableCell, TableRow } from './table'

interface EmptyStateProps {
  icon: LucideIcon
  title: string
  description?: string
  // Buttons or links that move the user forward: create the first entry, clear
  // the filter that matched nothing.
  children?: React.ReactNode
}

/**
 * What a list shows when it has nothing to show.
 *
 * A bare "No users found" leaves the operator guessing whether the directory is
 * empty, the search is too narrow, or something failed — so each caller
 * distinguishes "nothing here yet" from "nothing matches this filter" and
 * offers the corresponding way out.
 */
export function EmptyState({ icon: Icon, title, description, children }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center gap-3 px-4 py-12 text-center">
      <div className="rounded-full bg-muted p-3">
        <Icon className="h-6 w-6 text-muted-foreground" aria-hidden="true" />
      </div>
      <div className="space-y-1">
        <p className="font-medium">{title}</p>
        {description && <p className="text-sm text-muted-foreground">{description}</p>}
      </div>
      {children && <div className="flex flex-wrap items-center justify-center gap-2 pt-1">{children}</div>}
    </div>
  )
}

/** EmptyState placed inside a table body, spanning every column. */
export function TableEmptyState({ colSpan, ...props }: EmptyStateProps & { colSpan: number }) {
  return (
    <TableRow className="hover:bg-transparent">
      <TableCell colSpan={colSpan} className="p-0">
        <EmptyState {...props} />
      </TableCell>
    </TableRow>
  )
}
