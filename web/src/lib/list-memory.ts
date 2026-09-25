import { useEffect } from 'react'

/** The list pages a detail or create page can lead back to. */
export type ListPath = '/users' | '/groups' | '/netgroups' | '/sudo-roles' | '/password-policies'

// The last view of each list in this tab: its search, sort, page and filters,
// exactly as the list keeps them in its URL. In memory, not storage: it only
// has to outlive the trip to a detail page and back, and a reload that forgets
// it lands on the list's default view, which is what a fresh visit expects.
const remembered = new Map<ListPath, Record<string, unknown>>()

/** Called by a list page with its validated search, on every change. */
export function useRememberListSearch(path: ListPath, search: object) {
  useEffect(() => {
    remembered.set(path, { ...search })
  }, [path, search])
}

/** The view to return to, or undefined for the list's defaults. */
export function recallListSearch(path: ListPath): Record<string, unknown> | undefined {
  return remembered.get(path)
}
