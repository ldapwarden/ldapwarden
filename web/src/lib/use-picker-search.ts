import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import type { ListParams } from './api'
import { useDebounced } from './use-debounced'

/**
 * Candidates a picker shows at once. Small on purpose: a picker is something you
 * type into, not something you scroll.
 */
export const PICKER_LIMIT = 20

interface PickerOptions {
  /** Skip the request entirely, e.g. behind a permission or a closed module. */
  enabled?: boolean
  /** Sort field for the candidate order — 'displayName' for users, 'cn' elsewhere. */
  sort?: string
  /**
   * Extra rows to request beyond PICKER_LIMIT, for callers that drop candidates
   * client-side (already a member, the entry being edited). Without it, a term
   * whose first matches are all excluded shows an empty picker while the server
   * still reports matches.
   */
  headroom?: number
}

/**
 * Server-side search for a picker: the directory does the matching, the browser
 * receives at most a screenful.
 *
 * The pickers used to fetch the whole collection and filter it in memory. That
 * quietly broke past LDAP_SEARCH_SIZE_LIMIT (500 by default): the request came
 * back capped, `truncated` was ignored, and typing a name that existed beyond
 * the cap produced "no results" — the picker was searching its slice, not the
 * directory. This asks the endpoint the same way the list views do.
 *
 * `overflow` is what the term still matches but is not shown, so a caller can
 * say so rather than let the list look complete.
 */
export function usePickerSearch<T>(
  queryKey: readonly unknown[],
  list: (params: ListParams, signal?: AbortSignal) => Promise<{ data: T[]; total: number }>,
  options: PickerOptions = {},
) {
  const { enabled = true, sort, headroom = 0 } = options
  const [input, setInput] = useState('')
  const term = useDebounced(input)
  const limit = PICKER_LIMIT + headroom

  const query = useQuery({
    queryKey: [...queryKey, { search: term, sort, limit }],
    queryFn: ({ signal }) => list({ search: term, sort, order: 'asc', limit }, signal),
    enabled,
    // Keep the previous candidates on screen while the next term is in flight,
    // so the list does not blink empty between keystrokes.
    placeholderData: keepPreviousData,
    staleTime: 30_000,
  })

  const items = query.data?.data ?? []
  const total = query.data?.total ?? 0

  return {
    /** Bind to the picker's search box. */
    input,
    setInput,
    /** Clear the term, for when the picker closes. */
    reset: () => setInput(''),
    items,
    total,
    /** Matches not shown: the term needs narrowing. */
    overflow: Math.max(0, total - items.length),
    isFetching: query.isFetching,
  }
}
