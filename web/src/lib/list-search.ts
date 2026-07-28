export type SortDirection = 'asc' | 'desc'

export const PAGE_SIZES = [10, 25, 50, 100] as const
export const DEFAULT_PAGE_SIZE = 25

/**
 * The view state of a list page, as it lives in the URL.
 *
 * Every field is optional and absent when it holds its default, which keeps
 * `/users` clean while `/users?q=doe&sort=mail&dir=desc&page=2` stays a link
 * someone can paste into a ticket. Read the effective values with
 * `listSearchValues`.
 */
export interface ListSearch<S extends string = string> {
  q?: string
  sort?: S
  dir?: SortDirection
  page?: number
  size?: number
}

function asString(value: unknown): string | undefined {
  return typeof value === 'string' && value !== '' ? value : undefined
}

function asOneOf<T extends string>(value: unknown, allowed: readonly T[]): T | undefined {
  return typeof value === 'string' && (allowed as readonly string[]).includes(value)
    ? (value as T)
    : undefined
}

function asPage(value: unknown): number | undefined {
  const n = Number(value)
  return Number.isInteger(n) && n > 1 ? n : undefined
}

function asSize(value: unknown): number | undefined {
  const n = Number(value)
  return (PAGE_SIZES as readonly number[]).includes(n) && n !== DEFAULT_PAGE_SIZE ? n : undefined
}

export function asBoolean(value: unknown): boolean | undefined {
  if (value === true || value === 'true' || value === '1') return true
  return undefined
}

/**
 * Builds the `validateSearch` function for a list route. Unknown sort fields,
 * out-of-range page sizes and junk page numbers are dropped rather than
 * rejected: a hand-edited or stale URL should degrade to the default view, not
 * to an error screen.
 */
export function listSearchValidator<S extends string>(sortFields: readonly S[]) {
  return (raw: Record<string, unknown>): ListSearch<S> => ({
    q: asString(raw.q),
    sort: asOneOf(raw.sort, sortFields),
    dir: asOneOf(raw.dir, ['asc', 'desc'] as const),
    page: asPage(raw.page),
    size: asSize(raw.size),
  })
}

/**
 * Applies the defaults that `listSearchValidator` leaves out of the URL.
 *
 * `NoInfer` on the default keeps the sort union coming from the search params:
 * without it, passing `'cn'` would narrow `S` to that one literal and every
 * other sort field would fail to typecheck at the call site.
 */
export function listSearchValues<S extends string>(search: ListSearch<S>, defaultSort: NoInfer<S>) {
  return {
    q: search.q ?? '',
    sort: (search.sort ?? defaultSort) as S,
    dir: search.dir ?? ('asc' as SortDirection),
    page: search.page ?? 1,
    size: search.size ?? DEFAULT_PAGE_SIZE,
  }
}

/**
 * Next sort state for a header click: same column flips direction, a new column
 * starts ascending. Sorting also returns to page 1 — page 3 of the previous
 * ordering describes nothing.
 */
export function nextSort<S extends string>(
  current: { sort: S; dir: SortDirection },
  field: S,
): { sort: S; dir: SortDirection; page: undefined } {
  const dir: SortDirection = current.sort === field && current.dir === 'asc' ? 'desc' : 'asc'
  return { sort: field, dir, page: undefined }
}
