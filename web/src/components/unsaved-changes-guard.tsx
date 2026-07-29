import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { useUnsavedChangesBlocker } from '@/lib/form-sync'

/**
 * Guards a form with unsaved changes.
 *
 * Replaces a native `window.confirm`, which said "Leave without saving?" even
 * when the user was only switching tabs on the same record — the tab panels
 * unmount, so their edits really would be lost, but "leave" described nothing
 * the user was doing. Same dialog component as the rest of the app, with wording
 * that matches the actual move.
 */
export function UnsavedChangesGuard({ when }: { when: boolean }) {
  const { status, proceed, reset, leavingPage } = useUnsavedChangesBlocker(when)

  return (
    <ConfirmDialog
      open={status === 'blocked'}
      onOpenChange={(open) => {
        if (!open) reset?.()
      }}
      title={leavingPage ? 'Leave without saving?' : 'Discard your changes?'}
      description={
        leavingPage
          ? 'This form has changes that have not been saved. Leaving now discards them.'
          : 'This tab has changes that have not been saved. Switching tabs discards them.'
      }
      confirmLabel={leavingPage ? 'Leave' : 'Discard changes'}
      cancelLabel={leavingPage ? 'Stay on this page' : 'Keep editing'}
      onConfirm={() => proceed?.()}
    />
  )
}
