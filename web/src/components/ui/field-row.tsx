import { cn } from '@/lib/utils'

/**
 * A row of form fields that collapses to one column on a narrow screen.
 *
 * Every detail and creation form laid its side-by-side fields out with a bare
 * `grid grid-cols-2` (and, on user creation, `grid-cols-4`) — no responsive
 * prefix, so the columns held at any width. The nav bar had been given careful
 * mobile treatment; the forms had not, and at 375px a four-field row came out
 * at roughly 70px per input, wide enough to hide the tail of a DN, a home
 * directory or a Samba SID while it was being typed.
 *
 * Going through a component rather than fixing 29 class lists means the next
 * field row inherits the behaviour instead of re-deriving the bug.
 */
const COLUMNS = {
  // Written out in full because Tailwind scans source for literal class names;
  // an interpolated `sm:grid-cols-${n}` would never be generated.
  2: 'grid-cols-1 sm:grid-cols-2',
  3: 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3',
  4: 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-4',
} as const

export function FieldRow({
  cols = 2,
  className,
  children,
}: {
  cols?: keyof typeof COLUMNS
  className?: string
  children: React.ReactNode
}) {
  return <div className={cn('grid gap-4', COLUMNS[cols], className)}>{children}</div>
}
