import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Trash2 } from 'lucide-react'
import { runBulk, bulkSummary, warningsOf } from '@/lib/bulk'
import { BulkActionBar } from '@/components/bulk-action-bar'
import { BulkResultDialog, type BulkOutcome, type BulkWarning } from '@/components/bulk-result-dialog'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Button } from '@/components/ui/button'

/**
 * The selection toolbar for a list whose only bulk action is delete: the bar,
 * the confirmation, the progress and the per-item failure report.
 *
 * Groups had all of this inline; sudo roles and password policies had none of
 * it. Sharing it is what stops the four list views from drifting apart again.
 *
 * A `remove` that answers with `warnings` (groups and netgroups do: sudo names
 * them by string) has them collected and listed per item once the run is over.
 * Dropping them made a bulk delete the one path that left rules granting to
 * nobody without saying so.
 */
export function BulkDeleteActions<T extends { dn: string }>({
  selected,
  onClear,
  noun,
  label,
  remove,
  invalidateKey,
}: {
  selected: T[]
  onClear: () => void
  /** Singular noun for the summaries, e.g. "sudo role". */
  noun: string
  /** How to name an item in the failure list. */
  label: (item: T) => string
  remove: (item: T) => Promise<unknown>
  /** Query key prefix to invalidate once the run finishes. */
  invalidateKey: readonly unknown[]
}) {
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [running, setRunning] = useState(false)
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null)
  const [outcome, setOutcome] = useState<BulkOutcome<T> | null>(null)

  const count = selected.length

  const run = async () => {
    setRunning(true)
    setProgress({ done: 0, total: count })
    try {
      const warned: BulkWarning<T>[] = []
      const result = await runBulk(selected, async (item) => {
        const warnings = warningsOf(await remove(item))
        if (warnings.length > 0) warned.push({ item, warnings })
      }, {
        onProgress: (done, total) => setProgress({ done, total }),
      })
      queryClient.invalidateQueries({ queryKey: invalidateKey })
      const summary = bulkSummary('Deleted', noun, result)
      // The toast reports the shape; the dialog says which ones failed and why,
      // and what the ones that went through left behind.
      if (result.failed.length === 0) {
        toast.success(summary)
      } else {
        toast.error(summary)
      }
      if (result.failed.length > 0 || warned.length > 0) {
        setOutcome({ verb: 'Deleted', noun, succeeded: result.succeeded, failed: result.failed, warned })
      }
      onClear()
      setOpen(false)
    } finally {
      setRunning(false)
      setProgress(null)
    }
  }

  return (
    <>
      <BulkActionBar count={count} onClear={onClear} progress={progress}>
        <Button variant="destructive" size="sm" disabled={running} onClick={() => setOpen(true)}>
          <Trash2 className="h-4 w-4 mr-1" />
          Delete
        </Button>
      </BulkActionBar>

      <BulkResultDialog outcome={outcome} label={label} onClose={() => setOutcome(null)} />

      <ConfirmDialog
        open={open}
        onOpenChange={(next) => { if (!next) setOpen(false) }}
        title={`Delete ${noun}s`}
        description={`Delete ${count} selected ${noun}${count === 1 ? '' : 's'}? This permanently removes ${count === 1 ? 'it' : 'them'} from the directory and cannot be undone.`}
        confirmLabel={`Delete ${count}`}
        pendingLabel="Deleting..."
        isPending={running}
        onConfirm={run}
      />
    </>
  )
}
