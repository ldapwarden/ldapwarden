/**
 * "There are more matches than these" for a picker fed by a server-side search.
 *
 * Pickers show a bounded number of candidates. Saying nothing would make the
 * list read as complete, which is how the previous client-side filtering managed
 * to hide entries past the directory's search limit without anyone noticing.
 */
export function PickerOverflow({ count }: { count: number }) {
  if (count <= 0) return null

  return (
    <p className="text-xs text-muted-foreground">
      {count === 1
        ? '1 more match is not shown — refine the search to reach it.'
        : `${count} more matches are not shown — refine the search to reach them.`}
    </p>
  )
}
