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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

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
  /**
   * When set, the confirm button stays disabled until this exact text is typed.
   * For the actions that cannot be walked back — the same bar the rename flows
   * already set — so a stray Enter on the wrong row does not go through.
   */
  confirmText?: string
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
  confirmText,
  isPending = false,
  onConfirm,
}: ConfirmDialogProps) {
  const [typed, setTyped] = React.useState('')
  const inputId = React.useId()
  const confirmed = confirmText === undefined || typed === confirmText
  // A fresh dialog starts empty: text typed for one entry must not carry over
  // and pre-confirm the next.
  const handleOpenChange = (next: boolean) => {
    if (!next) setTyped('')
    onOpenChange?.(next)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      {trigger && <DialogTrigger asChild>{trigger}</DialogTrigger>}
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {confirmText !== undefined && (
          <div className="space-y-2">
            <Label htmlFor={inputId}>
              Type <span className="font-mono font-semibold">{confirmText}</span> to confirm
            </Label>
            <Input
              id={inputId}
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              autoComplete="off"
              spellCheck={false}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && confirmed && !isPending) onConfirm()
              }}
            />
          </div>
        )}
        {error && (
          <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md">
            {error}
          </div>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            {cancelLabel}
          </Button>
          <Button variant={confirmVariant} onClick={onConfirm} disabled={isPending || !confirmed}>
            {isPending ? pendingLabel : confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
