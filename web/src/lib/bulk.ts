export interface BulkFailure<T> {
  item: T
  error: string
}

export interface BulkResult<T> {
  succeeded: number
  failed: BulkFailure<T>[]
}

export interface BulkOptions {
  /** Requests in flight at once. */
  concurrency?: number
  /**
   * Called after each item settles, with how many of `total` are done.
   *
   * A bulk run is N sequential-ish requests, so a hundred-row page takes real
   * time; without this the UI could only grey the buttons out and leave the
   * operator watching nothing happen.
   */
  onProgress?: (done: number, total: number) => void
}

/**
 * Runs an async `worker` over `items` with a bounded number of concurrent
 * calls, collecting per-item failures instead of aborting on the first error.
 * Used to drive client-side bulk actions over the existing per-item API
 * endpoints (lock/unlock/delete/…) so one bad row doesn't sink the batch.
 */
export async function runBulk<T>(
  items: T[],
  worker: (item: T) => Promise<unknown>,
  { concurrency = 5, onProgress }: BulkOptions = {},
): Promise<BulkResult<T>> {
  let index = 0
  let succeeded = 0
  let settled = 0
  const failed: BulkFailure<T>[] = []

  async function next(): Promise<void> {
    const i = index++
    if (i >= items.length) return
    try {
      await worker(items[i])
      succeeded++
    } catch (e) {
      failed.push({ item: items[i], error: e instanceof Error ? e.message : String(e) })
    }
    settled++
    onProgress?.(settled, items.length)
    return next()
  }

  const workers = Array.from({ length: Math.min(concurrency, items.length) }, () => next())
  await Promise.all(workers)
  return { succeeded, failed }
}

/** "Locked 12 users." / "Locked 1 user, 2 failed." for a bulk result toast. */
export function bulkSummary(verb: string, noun: string, result: BulkResult<unknown>): string {
  // Pluralise on the count each word attaches to, not on the batch size:
  // "Deleted 1 users, 1 failed" was reachable with a batch of two.
  const unit = `${noun}${result.succeeded === 1 ? '' : 's'}`
  if (result.failed.length === 0) return `${verb} ${result.succeeded} ${unit}.`
  return `${verb} ${result.succeeded} ${unit}, ${result.failed.length} failed.`
}
