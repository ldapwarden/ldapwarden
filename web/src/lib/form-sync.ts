import { useEffect, useRef, useState } from 'react'
import { useBlocker } from '@tanstack/react-router'

/**
 * Keeps a detail form in sync with server data fetched via react-query and
 * reports whether the form has unsaved edits.
 *
 * Detail pages initialise their form state from the query result, but useState
 * only reads its initial value once. On a cold load (data not yet cached) the
 * query is still pending at mount, so the form would initialise empty and never
 * pick up the data that arrives a moment later. This adopts the server data on
 * first arrival and on any later change, but ONLY while the form is "pristine"
 * (unchanged since the last sync), so edits in progress are never clobbered by
 * a background refetch.
 *
 * `serverForm` must be shaped exactly like `form` (map the raw API object down
 * to the editable fields before passing it in), so equality checks compare
 * like with like.
 */
export function useSyncedForm<T>(
  serverForm: T | undefined,
  form: T,
  setForm: (value: T) => void,
): boolean {
  const lastSynced = useRef<string | null>(null)
  const serverJSON = serverForm === undefined ? null : JSON.stringify(serverForm)

  useEffect(() => {
    if (serverForm === undefined || serverJSON === null) return
    const pristine =
      lastSynced.current === null || JSON.stringify(form) === lastSynced.current
    if (serverJSON !== lastSynced.current && pristine) {
      setForm(serverForm)
      lastSynced.current = serverJSON
    }
    // Intentionally keyed on serverJSON only: we react to server changes, and
    // read the current form via closure. Including `form` would re-run on every
    // keystroke (harmless, but pointless).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverJSON])

  return serverJSON !== null && JSON.stringify(form) !== serverJSON
}

/**
 * Whether a creation form has been touched, for arming UnsavedChangesGuard.
 *
 * A detail form compares itself to the server's copy (useSyncedForm); a creation
 * form has no server copy, so it compares against the values it held on its
 * first render. Snapshotting rather than taking the initial object as a second
 * argument keeps the two from drifting apart — which matters because the
 * defaults are spread across a dozen fields. It does assume nothing populates
 * the form asynchronously; a form that adopts fetched defaults would read as
 * edited the moment they land.
 *
 * `settled` disarms the guard: once the record has been created the page
 * navigates to it, and that navigation must not be blocked by the very values
 * that were just saved.
 */
export function useCreationFormDirty<T>(current: T, settled: boolean): boolean {
  const currentJSON = JSON.stringify(current)
  // State, not a ref: the initial value has to be readable during render, and
  // useState's initialiser is exactly "the value from the first render".
  const [initialJSON] = useState(currentJSON)
  return !settled && currentJSON !== initialJSON
}

/**
 * Blocks in-app navigation away from a form with unsaved changes, and arms the
 * native beforeUnload dialog for closing/reloading the tab. No-op while `when`
 * is false.
 *
 * Returns the blocker state so the caller can render the app's own dialog:
 * `status` is 'blocked' while a navigation is held, `proceed` discards the edits
 * and continues, `reset` stays put. `leavingPage` distinguishes moving to
 * another page from switching tabs within the same record — the wording differs,
 * and "leave without saving?" is simply wrong for a tab switch.
 */
export function useUnsavedChangesBlocker(when: boolean) {
  const blocker = useBlocker({
    shouldBlockFn: ({ current, next }) => {
      if (!when) return false
      // Same route and same params, only the search changed → a tab switch.
      lastBlockWasSamePage.current =
        current.routeId === next.routeId &&
        JSON.stringify(current.params) === JSON.stringify(next.params)
      return true
    },
    withResolver: true,
    enableBeforeUnload: when,
  })

  return {
    status: blocker.status,
    proceed: blocker.proceed,
    reset: blocker.reset,
    leavingPage: !lastBlockWasSamePage.current,
  }
}

// Set by shouldBlockFn, read when rendering the dialog. A module-level ref
// rather than state: it must be readable in the same render that flips
// `status` to 'blocked', and only one form can be blocked at a time.
const lastBlockWasSamePage = { current: false }
