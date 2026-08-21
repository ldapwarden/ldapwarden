import * as React from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'

interface ConfirmDialogProps {
  /** Controlled open state. Omit when relying solely on `trigger`. */
  open?: boolean
  onOpenChange?: (open: boolean) => void
  /** Optional element that opens the dialog (wrapped in DialogTrigger). */
  trigger?: React.ReactNode
  title: string
  description: React.ReactNode
  confirmLabel?: string
  pendingLabel?: string
  cancelLabel?: string
  /**
   * Emphasis of the confirm button. Destructive by default — this dialog grew
   * out of the delete flows — but a reversible bulk action (unlocking a page of
   * accounts) still deserves a confirmation without a red button implying
   * something is about to be destroyed.
   */
  confirmVariant?: 'destructive' | 'default'
  /** Error message shown inside the dialog (e.g. a failed mutation). */
  error?: string | null
  isPending?: boolean
  onConfirm: () => void
}

/**
 * A confirmation dialog for destructive actions. Centralises the
 * title/description/error/confirm-button pattern that was previously
 * duplicated across every delete flow.
 */
export function ConfirmDialog({
  open,
  onOpenChange,
  trigger,
  title,
  description,
  confirmLabel = 'Delete',
  pendingLabel = 'Deleting...',
  cancelLabel = 'Cancel',
  confirmVariant = 'destructive',
  error,
  isPending = false,
  onConfirm,
}: ConfirmDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {trigger && <DialogTrigger asChild>{trigger}</DialogTrigger>}
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {error && (
          <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md">
            {error}
          </div>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange?.(false)}>
            {cancelLabel}
          </Button>
          <Button variant={confirmVariant} onClick={onConfirm} disabled={isPending}>
            {isPending ? pendingLabel : confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
