import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Callout } from '@/components/ui/callout'
import type { BulkFailure } from '@/lib/bulk'

export interface BulkOutcome<T> {
  verb: string
  noun: string
  succeeded: number
  failed: BulkFailure<T>[]
  /**
   * Items that succeeded but left something behind the operator must act on
   * (a sudo rule still naming a deleted netgroup, say). Listed here rather than
   * in a toast: in a batch, a toast is gone before anyone has read it.
   */
  warned?: BulkWarning<T>[]
}

export interface BulkWarning<T> {
  item: T
  warnings: string[]
}

/**
 * The per-item outcome of a bulk action.
 *
 * `runBulk` already collects every failure with its item and message, but the
 * report was a toast carrying counts and *one* error — so a run of 50 with 7
 * failures told you neither which 7 nor why. This lists them, keeping the toast
 * for the "everything worked" case where there is nothing to inspect.
 */
export function BulkResultDialog<T>({
  outcome,
  label,
  onClose,
}: {
  outcome: BulkOutcome<T> | null
  // How to name an item in the failure list (uid, cn, …).
  label: (item: T) => string
  onClose: () => void
}) {
  if (!outcome) return null

  const total = outcome.succeeded + outcome.failed.length
  const warned = outcome.warned ?? []

  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose() }}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {outcome.verb} {outcome.succeeded} of {total} {outcome.noun}
            {total === 1 ? '' : 's'}
          </DialogTitle>
          <DialogDescription>
            {outcome.failed.length > 0 && (
              <>
                {outcome.failed.length === 1 ? '1 item failed' : `${outcome.failed.length} items failed`}
                {outcome.succeeded > 0
                  ? `${outcome.failed.length === 1 ? ' and was' : ' and were'} left unchanged. Each item is applied independently, so the ones that worked are already done.`
                  : ' — nothing was changed.'}
              </>
            )}
            {outcome.failed.length > 0 && warned.length > 0 && ' '}
            {warned.length > 0 &&
              `${warned.length === 1 ? '1 item' : `${warned.length} items`} went through but left references behind that the directory will not clean up on its own.`}
          </DialogDescription>
        </DialogHeader>

        <div className="max-h-72 space-y-2 overflow-y-auto">
          {outcome.failed.map((failure, index) => (
            <Callout key={`failed-${index}`} variant="error" icon={false}>
              <p className="font-medium">{label(failure.item)}</p>
              <p className="text-muted-foreground">{failure.error}</p>
            </Callout>
          ))}
          {warned.map((entry, index) => (
            <Callout key={`warned-${index}`} variant="warning" icon={false}>
              <p className="font-medium">{label(entry.item)}</p>
              <ul className="list-disc pl-4 text-muted-foreground">
                {entry.warnings.map((warning, i) => (
                  <li key={i}>{warning}</li>
                ))}
              </ul>
            </Callout>
          ))}
        </div>

        <DialogFooter>
          <Button onClick={onClose}>Close</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
