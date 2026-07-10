import { Lock } from 'lucide-react'

/**
 * Shown at the top of a detail form when the current user lacks write
 * permission. The fields below are already disabled; this explains why, so a
 * greyed-out form doesn't read as broken.
 */
export function ReadOnlyNotice() {
  return (
    <div className="flex items-center gap-2 rounded-md border bg-muted px-3 py-2 text-sm text-muted-foreground">
      <Lock className="h-4 w-4 shrink-0" />
      <span>You have read-only access — fields are shown for reference and can't be edited.</span>
    </div>
  )
}
