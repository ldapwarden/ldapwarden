import { useCallback, useMemo, useState } from 'react'

/**
 * Checkbox selection for a list page, keyed by DN.
 *
 * Deliberately scoped to the rows on screen: paging is done by the directory, so
 * the rest of the result set is not in memory, and a bulk action must never
 * reach rows the operator has not seen.
 *
 * Extracted because the users and groups lists each grew their own copy while
 * sudo roles and password policies grew a different interaction model instead —
 * per-row icons and no selection — so deleting five stale sudo roles took five
 * dialogs. One implementation is what keeps the four lists behaving alike.
 */
export function useRowSelection<T extends { dn: string }>(rows: T[]) {
  const [selected, setSelected] = useState<Set<string>>(new Set())

  const selectedRows = useMemo(() => rows.filter((row) => selected.has(row.dn)), [rows, selected])
  const allSelected = rows.length > 0 && rows.every((row) => selected.has(row.dn))
  const someSelected = selected.size > 0 && !allSelected

  const toggleOne = useCallback((dn: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(dn)) next.delete(dn)
      else next.add(dn)
      return next
    })
  }, [])

  const toggleAll = useCallback(() => {
    setSelected((prev) =>
      rows.every((row) => prev.has(row.dn)) ? new Set() : new Set(rows.map((row) => row.dn)),
    )
  }, [rows])

  const clear = useCallback(() => setSelected(new Set()), [])

  return { selected, selectedRows, allSelected, someSelected, toggleOne, toggleAll, clear }
}
