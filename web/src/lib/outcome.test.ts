import { afterEach, describe, expect, it } from 'vitest'
import { currentOutcome, dismissOutcome, outcomeText, showOutcome } from './outcome'

afterEach(() => {
  while (currentOutcome()) dismissOutcome()
})

describe('outcome queue', () => {
  it('ignores an outcome with nothing to report', () => {
    // Callers pass res.warnings unconditionally; an empty list must not open
    // an empty dialog after every clean rename.
    showOutcome({ title: 'Group renamed', items: [] })
    expect(currentOutcome()).toBeNull()
  })

  it('queues a second outcome behind the first instead of replacing it', () => {
    showOutcome({ title: 'first', items: ['a'] })
    showOutcome({ title: 'second', items: ['b'] })
    expect(currentOutcome()?.title).toBe('first')
    dismissOutcome()
    expect(currentOutcome()?.title).toBe('second')
    dismissOutcome()
    expect(currentOutcome()).toBeNull()
  })

  it('copies as plain text, one item per line', () => {
    expect(outcomeText({ title: 'POSIX numbers changed', items: ['chown on web01', 'chown on db01'] }))
      .toBe('POSIX numbers changed\n\n- chown on web01\n- chown on db01')
  })
})
