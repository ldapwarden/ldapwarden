import { toast } from 'sonner'
import { Copy } from 'lucide-react'
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
import { dismissOutcome, outcomeText, useCurrentOutcome } from '@/lib/outcome'

/**
 * Renders the queued outcome (see lib/outcome.ts). Mounted once, at the root,
 * so it outlives the navigation that usually follows the operation.
 *
 * A click outside does not dismiss it: this is a list of things still to do,
 * and losing it to a stray click is the failure it exists to prevent. Escape,
 * the close button and "Done" are all deliberate.
 */
export function OutcomeHost() {
  const outcome = useCurrentOutcome()
  if (!outcome) return null

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(outcomeText(outcome))
      toast.success('Copied to the clipboard')
    } catch {
      toast.error('Could not copy: the browser refused clipboard access')
    }
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open) dismissOutcome() }}>
      <DialogContent className="sm:max-w-xl" onInteractOutside={(e) => e.preventDefault()}>
        <DialogHeader>
          <DialogTitle>{outcome.title}</DialogTitle>
          {/* Always present: Radix wants a description, and the default says why
              this did not just vanish like a toast. */}
          <DialogDescription>
            {outcome.description ?? 'The change went through. What follows is left for you to check or do.'}
          </DialogDescription>
        </DialogHeader>

        <Callout variant={outcome.tone ?? 'warning'} icon={false}>
          <ul className="list-disc space-y-1 pl-4">
            {outcome.items.map((item, index) => (
              <li key={index} className="break-words">{item}</li>
            ))}
          </ul>
        </Callout>

        <DialogFooter className="gap-2">
          <Button variant="outline" onClick={copy}>
            <Copy className="h-4 w-4 mr-1" />
            Copy
          </Button>
          <Button onClick={dismissOutcome}>Done</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
