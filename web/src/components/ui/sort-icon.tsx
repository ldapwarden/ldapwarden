import { ArrowUp, ArrowDown, ArrowUpDown } from 'lucide-react'

export function SortIcon({ field, sortField, sortDirection }: {
  field: string
  sortField: string
  sortDirection: 'asc' | 'desc'
}) {
  if (sortField !== field) return <ArrowUpDown className="ml-1 h-4 w-4 text-muted-foreground" />
  return sortDirection === 'asc'
    ? <ArrowUp className="ml-1 h-4 w-4" />
    : <ArrowDown className="ml-1 h-4 w-4" />
}
