import type { ReactNode } from 'react'
import { Save, Undo2 } from 'lucide-react'
import { Button } from '@/components/ui/button'

/**
 * The Discard / Save pair at the foot of a detail form.
 *
 * Save used to be enabled whatever the state of the form, so saving an
 * untouched form sent an empty change and answered "updated successfully" — a
 * confirmation of nothing. Every detail form already knew whether it was dirty
 * (the unsaved-changes guard reads it); this puts that on the button, and
 * offers the way back to the stored values that only a reload gave before.
 *
 * Renders the buttons only: the caller keeps its own row, so a form that has a
 * Delete on the other side, or a Cancel for an add-object-class mode, keeps it.
 */
export function FormActions({
  dirty,
  pending,
  onDiscard,
  submit,
  children,
}: {
  dirty: boolean
  pending: boolean
  /** Puts the stored values back. Omitted where there is nothing to go back to. */
  onDiscard?: () => void
  /**
   * Called on Save for a form without a <form> element; when absent the button
   * is a submit button and the enclosing form handles it.
   */
  submit?: () => void
  /** Extra buttons placed before Discard, e.g. a Cancel. */
  children?: ReactNode
}) {
  return (
    <>
      {children}
      {onDiscard && dirty && (
        <Button type="button" variant="outline" disabled={pending} onClick={onDiscard}>
          <Undo2 className="h-4 w-4 mr-1" />
          Discard
        </Button>
      )}
      <Button
        type={submit ? 'button' : 'submit'}
        onClick={submit}
        disabled={!dirty || pending}
        title={!dirty && !pending ? 'No changes to save' : undefined}
      >
        <Save className="h-4 w-4 mr-1" />
        {pending ? 'Saving...' : 'Save Changes'}
      </Button>
    </>
  )
}
