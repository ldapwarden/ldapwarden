import { Link } from '@tanstack/react-router'
import { ArrowLeft } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { recallListSearch, type ListPath } from '@/lib/list-memory'

/**
 * The back arrow of a detail or create page.
 *
 * It returns to the list as it was left — search, sort, page, filter — rather
 * than to the list's defaults: the list keeps its view in the URL precisely so
 * it survives, and a back arrow that dropped it undid that on every drill-down.
 * A link rather than a click handler, so it can be opened in a new tab.
 */
export function BackToList({ to, label }: { to: ListPath; label: string }) {
  return (
    <Button variant="ghost" size="icon" asChild>
      {/* The router types `search` per route; the remembered value is the
          list's own validated search, so the cast only restates that. */}
      <Link to={to} search={recallListSearch(to) as never} aria-label={label}>
        <ArrowLeft className="h-4 w-4" />
      </Link>
    </Button>
  )
}
