import { X } from 'lucide-react'

/**
 * A list of values shown as removable chips.
 *
 * Extracted from the two sudo-role pages, which each carried their own copy —
 * differing only in the empty-state wording and in whether they honoured a
 * read-only session. The netgroup editors would have made a third.
 */
export function MultiValueField({
  values,
  onRemove,
  canWrite = true,
  mono = false,
  empty = 'None configured',
  label,
}: {
  values: string[]
  onRemove: (value: string) => void
  canWrite?: boolean
  mono?: boolean
  /** Shown instead of the chips when there is nothing in the list. */
  empty?: string
  /** Names each remove button for a screen reader, e.g. "user" → "Remove user jdoe". */
  label?: string
}) {
  if (values.length === 0) {
    return <p className="text-sm text-muted-foreground">{empty}</p>
  }

  return (
    <div className="flex flex-wrap gap-2">
      {values.map((value, index) => (
        <span
          key={index}
          className={`inline-flex items-center gap-1 px-2 py-1 rounded-md bg-muted text-sm ${mono ? 'font-mono' : ''}`}
        >
          {value}
          {canWrite && (
            <button
              type="button"
              onClick={() => onRemove(value)}
              aria-label={label ? `Remove ${label} ${value}` : `Remove ${value}`}
              className="hover:text-destructive"
            >
              <X className="h-3 w-3" aria-hidden="true" />
            </button>
          )}
        </span>
      ))}
    </div>
  )
}
