import { Link } from '@tanstack/react-router'
import { SearchX } from 'lucide-react'
import { ApiError } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Callout } from '@/components/ui/callout'
import { BackToList } from '@/components/back-to-list'
import { recallListSearch, type ListPath } from '@/lib/list-memory'

/**
 * What a detail page shows when its entry cannot be loaded.
 *
 * A bare red "Failed to load user: user not found" with no way out was the
 * usual end of a stale link — to an entry renamed (the DN is the address, so a
 * rename moves it) or deleted since. That case gets its own explanation and a
 * way back; anything else is reported as the failure it is.
 */
export function DetailLoadError({
  error,
  noun,
  list,
  listLabel,
}: {
  error: Error | null
  /** "user", "group", … */
  noun: string
  list: ListPath
  /** "users", "groups", … */
  listLabel: string
}) {
  // 400 is a DN the server could not parse or that falls outside the base: a
  // malformed link, which for the reader amounts to the same thing.
  const missing = error instanceof ApiError && (error.status === 404 || error.status === 400)

  if (!missing) {
    return (
      <div className="space-y-4">
        <BackToList to={list} label={`Back to ${listLabel}`} />
        <Callout variant="error">
          Failed to load {noun}: {error?.message ?? 'no data returned'}
        </Callout>
      </div>
    )
  }

  return (
    <div className="flex flex-col items-center justify-center py-16 text-center">
      <div className="mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-muted">
        <SearchX className="h-6 w-6 text-muted-foreground" aria-hidden="true" />
      </div>
      <h1 className="text-2xl font-bold">This {noun} does not exist</h1>
      <p className="mt-2 max-w-md text-sm text-muted-foreground">
        The link may be older than a rename — an entry&apos;s address is its DN, so renaming it
        moves it — or the {noun} was deleted since. Search the list for it.
      </p>
      <Button asChild className="mt-6">
        <Link to={list} search={recallListSearch(list) as never}>
          Back to {listLabel}
        </Link>
      </Button>
    </div>
  )
}
