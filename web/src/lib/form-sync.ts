import { useEffect, useRef } from 'react'
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
 * Warns before leaving a page with unsaved changes — both for in-app
 * navigation (router blocker with a confirm prompt) and for closing/reloading
 * the tab (native beforeUnload dialog). No-op while `when` is false.
 */
export function useUnsavedChangesPrompt(when: boolean) {
  useBlocker({
    shouldBlockFn: () => {
      if (!when) return false
      return !window.confirm('You have unsaved changes. Leave without saving?')
    },
    enableBeforeUnload: when,
  })
}
