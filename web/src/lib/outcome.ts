import { useSyncExternalStore } from 'react'

/**
 * What an operation left for the operator to do: references it could not
 * migrate, files to chown on the hosts, a probe entry to delete by hand.
 *
 * These used to be toasts, and a toast is the wrong vehicle for a to-do list:
 * it is gone in a few seconds, usually while the page navigates away (a delete
 * lands on the list, a rename on the new DN). An outcome stays on screen until
 * it is dismissed, survives that navigation because it is rendered at the root,
 * and can be copied.
 */
export interface Outcome {
  title: string
  description?: string
  items: string[]
  /** `error` when something that should have happened did not. */
  tone?: 'warning' | 'error'
}

// A queue, not a slot: two operations can finish close together (a rename that
// warns, then a task run that errors), and the second must not replace the first
// before anyone has read it.
let queue: Outcome[] = []
const listeners = new Set<() => void>()

function emit() {
  for (const listener of listeners) listener()
}

/** Shows an outcome once nothing else is. A call with no items is a no-op. */
export function showOutcome(outcome: Outcome) {
  if (outcome.items.length === 0) return
  queue = [...queue, outcome]
  emit()
}

/** Dismisses the outcome on screen, revealing the next one if any. */
export function dismissOutcome() {
  queue = queue.slice(1)
  emit()
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

/** The outcome to show now, or null. */
export function currentOutcome(): Outcome | null {
  return queue[0] ?? null
}

/** currentOutcome, re-rendering when it changes. */
export function useCurrentOutcome(): Outcome | null {
  return useSyncExternalStore(subscribe, currentOutcome, currentOutcome)
}

/** The outcome as plain text, one item per line, for the clipboard. */
export function outcomeText(outcome: Outcome): string {
  return [outcome.title, '', ...outcome.items.map((item) => `- ${item}`)].join('\n')
}
