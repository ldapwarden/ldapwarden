import type { DirectoryChangeOrigin } from './api'

// Filters of the audit page's "In the directory" view, shared with the route's
// validateSearch.

export type OriginFilter = DirectoryChangeOrigin | 'all'

export const ORIGIN_OPTIONS: { value: OriginFilter; label: string }[] = [
  { value: 'external', label: 'Outside ldapwarden' },
  { value: 'ldapwarden', label: 'Through ldapwarden' },
  { value: 'policy', label: 'Password policy' },
  { value: 'all', label: 'All writes' },
]

export const CHANGE_TYPE_OPTIONS = [
  { value: '', label: 'All operations' },
  { value: 'add', label: 'add' },
  { value: 'modify', label: 'modify' },
  { value: 'delete', label: 'delete' },
  { value: 'modrdn', label: 'modrdn' },
]

// The view state this tab keeps in the URL, alongside the audit page's own.
export interface DirectoryChangesSearch {
  origin?: OriginFilter
  type?: string
  q?: string
  from?: string
  to?: string
  page?: number
  size?: number
}
