import { describe, expect, it } from 'vitest'
import { bulkSummary, runBulk, warningsOf } from './bulk'

describe('warningsOf', () => {
  it('returns the warnings a delete endpoint answered with', () => {
    expect(warningsOf({ message: 'group deleted', warnings: ['sudo role "ops" still grants to %eng'] }))
      .toEqual(['sudo role "ops" still grants to %eng'])
  })

  it('returns nothing for endpoints that do not report leftovers', () => {
    expect(warningsOf(undefined)).toEqual([])
    expect(warningsOf(null)).toEqual([])
    expect(warningsOf({ message: 'sudo role deleted' })).toEqual([])
    expect(warningsOf({ message: 'group deleted', warnings: [] })).toEqual([])
  })

  it('ignores a malformed warnings field rather than throwing', () => {
    expect(warningsOf({ warnings: 'not a list' })).toEqual([])
    expect(warningsOf({ warnings: ['kept', 42, null] })).toEqual(['kept'])
  })
})

describe('runBulk', () => {
  it('collects failures per item without stopping the batch', async () => {
    const result = await runBulk([1, 2, 3], async (n) => {
      if (n === 2) throw new Error('boom')
    })
    expect(result.succeeded).toBe(2)
    expect(result.failed).toEqual([{ item: 2, error: 'boom' }])
    expect(bulkSummary('Deleted', 'group', result)).toBe('Deleted 2 groups, 1 failed.')
  })
})
