import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import * as DialogPrimitive from '@radix-ui/react-dialog'
import {
  KeyRound,
  LayoutDashboard,
  Plus,
  Search,
  Settings,
  ShieldCheck,
  Upload,
  User,
  Users,
  UsersRound,
  Network,
} from 'lucide-react'
import { api } from '@/lib/api'
import { useIsAdmin, useSections, type Section } from '@/lib/sections'
import { useDebounced } from '@/lib/use-debounced'
import { AccountStatus } from '@/components/account-status'
import type { AccountStatusFields } from '@/lib/account'
import { encodeDN } from '@/lib/utils'
import { cn } from '@/lib/utils'

interface Command {
  id: string
  label: string
  hint?: string
  group: string
  icon: typeof Users
  /** Present on user rows, so a locked account is flagged where it is found. */
  account?: AccountStatusFields
  run: () => void
}

/**
 * Cmd/Ctrl+K palette: type a few letters, jump straight to a user, group, sudo
 * role or policy — or to a page or a create form.
 *
 * On a directory tool the same lookup ("where is jdoe") happens dozens of times
 * a day, and doing it through the nav plus a list search plus a click is three
 * interactions too many.
 */
export function CommandPalette({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const navigate = useNavigate()
  const sections = useSections()
  const isAdmin = useIsAdmin()
  const [query, setQuery] = useState('')
  const debouncedQuery = useDebounced(query)
  const [activeIndex, setActiveIndex] = useState(0)
  const listRef = useRef<HTMLDivElement>(null)

  // A section this session cannot open is not searched either: a hidden
  // netgroups module still answered 501 to a search on every keystroke.
  const has = (key: Section['key']) => sections.some((section) => section.key === key)
  const searchUsers = has('users')
  const searchGroups = has('groups')
  const searchNetgroups = has('netgroups')
  const searchSudo = has('sudo')
  const searchPolicies = has('policies')

  // Reset on open (a stale query from last time is never what you want) and on a
  // new term (the highlight must land on a row that exists). Both are done
  // during render, which is React's way to adjust state when an input changes —
  // an effect would cost an extra render and a visible flicker of stale rows.
  const [lastOpen, setLastOpen] = useState(open)
  if (open !== lastOpen) {
    setLastOpen(open)
    if (open) {
      setQuery('')
      setActiveIndex(0)
    }
  }

  const term = debouncedQuery.trim()
  const searching = term.length > 0
  // Each directory search is capped: the palette is for jumping, not browsing.
  const perGroupLimit = 5

  const { data: users } = useQuery({
    queryKey: ['users', { search: term, limit: perGroupLimit, palette: true }],
    queryFn: ({ signal }) => api.users.list({ search: term, limit: perGroupLimit }, signal),
    enabled: open && searching && searchUsers,
  })

  const { data: groups } = useQuery({
    queryKey: ['groups', { search: term, limit: perGroupLimit, palette: true }],
    queryFn: ({ signal }) => api.groups.list({ search: term, limit: perGroupLimit }, signal),
    enabled: open && searching && searchGroups,
  })

  const { data: netgroups } = useQuery({
    queryKey: ['netgroups', { search: term, palette: true }],
    queryFn: ({ signal }) => api.netgroups.list({ search: term, limit: perGroupLimit }, signal),
    enabled: open && searching && searchNetgroups,
  })

  const { data: sudoRoles } = useQuery({
    queryKey: ['sudo-roles', { search: term, palette: true }],
    queryFn: ({ signal }) => api.sudoRoles.list({ search: term, limit: perGroupLimit }, signal),
    enabled: open && searching && searchSudo,
  })

  const { data: policies } = useQuery({
    queryKey: ['password-policies', { search: term, palette: true }],
    queryFn: ({ signal }) => api.passwordPolicies.list({ search: term, limit: perGroupLimit }, signal),
    enabled: open && searching && searchPolicies,
  })

  const commands = useMemo<Command[]>(() => {
    const go = (to: string, params?: Record<string, string>) => () => {
      onOpenChange(false)
      // The router's typing wants a literal route; the palette builds them
      // dynamically, so this is the one place that needs the cast.
      navigate({ to, params } as never)
    }

    const items: Command[] = []

    if (searching) {
      for (const user of users?.data ?? []) {
        items.push({
          id: `user:${user.dn}`,
          label: user.displayName || user.cn,
          hint: user.uid,
          group: 'Users',
          icon: User,
          account: user,
          run: go('/users/$dn', { dn: encodeDN(user.dn) }),
        })
      }
      for (const group of groups?.data ?? []) {
        items.push({
          id: `group:${group.dn}`,
          label: group.cn,
          hint: group.description || `gid ${group.gidNumber}`,
          group: 'Groups',
          icon: UsersRound,
          run: go('/groups/$dn', { dn: encodeDN(group.dn) }),
        })
      }
      for (const netgroup of netgroups?.data ?? []) {
        items.push({
          id: `netgroup:${netgroup.dn}`,
          label: netgroup.cn,
          hint: netgroup.description,
          group: 'Netgroups',
          icon: Network,
          run: go('/netgroups/$dn', { dn: encodeDN(netgroup.dn) }),
        })
      }
      for (const role of sudoRoles?.data ?? []) {
        items.push({
          id: `sudo:${role.dn}`,
          label: role.cn,
          hint: role.description,
          group: 'Sudo roles',
          icon: ShieldCheck,
          run: go('/sudo-roles/$dn', { dn: encodeDN(role.dn) }),
        })
      }
      for (const policy of policies?.data ?? []) {
        items.push({
          id: `policy:${policy.dn}`,
          label: policy.cn,
          hint: policy.description,
          group: 'Password policies',
          icon: KeyRound,
          run: go('/password-policies/$dn', { dn: encodeDN(policy.dn) }),
        })
      }
    }

    // Navigation and create actions, from the same list as the nav bar so the
    // two cannot disagree about what exists. Shown on their own when the palette
    // opens, and filtered by the term once the user starts typing, so "new"
    // reaches the create forms without leaving the keyboard.
    const actions: Command[] = [
      { id: 'go:dashboard', label: 'Dashboard', group: 'Go to', icon: LayoutDashboard, run: go('/') },
      ...sections.map((section) => ({
        id: `go:${section.key}`,
        label: section.label,
        group: 'Go to',
        icon: section.icon,
        run: go(section.to),
      })),
    ]
    if (isAdmin) {
      actions.push({ id: 'go:admin', label: 'Administration', group: 'Go to', icon: Settings, run: go('/admin') })
    }
    for (const section of sections) {
      if (section.create) {
        actions.push({ id: `new:${section.key}`, label: section.create.label, group: 'Create', icon: Plus, run: go(section.create.to) })
      }
    }
    for (const section of sections) {
      if (section.import) {
        actions.push({ id: `import:${section.key}`, label: section.import.label, group: 'Create', icon: Upload, run: go(section.import.to) })
      }
    }

    const needle = term.toLowerCase()
    items.push(...actions.filter((action) => !searching || action.label.toLowerCase().includes(needle)))

    return items
  }, [
    searching, term, users, groups, sudoRoles, policies, netgroups,
    sections, isAdmin, navigate, onOpenChange,
  ])

  const [lastTerm, setLastTerm] = useState(term)
  if (term !== lastTerm) {
    setLastTerm(term)
    setActiveIndex(0)
  }

  // Results also change as data arrives; keep the highlight in range.
  const boundedIndex = Math.min(activeIndex, Math.max(0, commands.length - 1))

  // Keep the highlighted row in view when moving with the arrow keys.
  useEffect(() => {
    listRef.current?.querySelector('[data-active="true"]')?.scrollIntoView({ block: 'nearest' })
  }, [boundedIndex])

  const onKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      setActiveIndex((i) => (commands.length === 0 ? 0 : (i + 1) % commands.length))
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      setActiveIndex((i) => (commands.length === 0 ? 0 : (i - 1 + commands.length) % commands.length))
    } else if (event.key === 'Enter') {
      event.preventDefault()
      commands[boundedIndex]?.run()
    }
  }

  // Group headings, in the order the commands were built.
  const groupsInOrder: string[] = []
  for (const command of commands) {
    if (!groupsInOrder.includes(command.group)) groupsInOrder.push(command.group)
  }

  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="fixed inset-0 z-50 bg-black/60" />
        <DialogPrimitive.Content
          onKeyDown={onKeyDown}
          className="fixed left-[50%] top-[15%] z-50 w-full max-w-xl translate-x-[-50%] overflow-hidden rounded-lg border bg-background shadow-lg"
        >
          <DialogPrimitive.Title className="sr-only">Search and commands</DialogPrimitive.Title>
          <DialogPrimitive.Description className="sr-only">
            Type to search users, groups, sudo roles and policies, or to run a command. Move with the
            arrow keys and press Enter.
          </DialogPrimitive.Description>

          <div className="flex items-center gap-2 border-b px-3">
            <Search className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
            {/* Autofocus is the point: the palette exists to be typed into. */}
            <input
              autoFocus
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search users, groups, roles… or type a command"
              aria-label="Search and commands"
              aria-activedescendant={commands[boundedIndex]?.id}
              aria-controls="command-palette-list"
              className="h-12 w-full bg-transparent text-sm outline-hidden placeholder:text-muted-foreground"
            />
          </div>

          <div ref={listRef} id="command-palette-list" role="listbox" className="max-h-80 overflow-y-auto p-1">
            {commands.length === 0 ? (
              <p className="px-3 py-6 text-center text-sm text-muted-foreground">
                {searching ? `Nothing matches “${term}”` : 'Start typing to search the directory'}
              </p>
            ) : (
              groupsInOrder.map((group) => (
                <div key={group} className="pb-1">
                  <p className="px-3 py-1.5 text-xs font-medium text-muted-foreground">{group}</p>
                  {commands
                    .map((command, index) => ({ command, index }))
                    .filter(({ command }) => command.group === group)
                    .map(({ command, index }) => {
                      const Icon = command.icon
                      const isActive = index === boundedIndex
                      return (
                        <button
                          key={command.id}
                          id={command.id}
                          type="button"
                          role="option"
                          aria-selected={isActive}
                          data-active={isActive}
                          onMouseMove={() => setActiveIndex(index)}
                          onClick={command.run}
                          className={cn(
                            'flex w-full items-center gap-2 rounded-md px-3 py-2 text-left text-sm',
                            isActive ? 'bg-accent text-accent-foreground' : 'hover:bg-accent/50',
                          )}
                        >
                          <Icon className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                          <span className="truncate">{command.label}</span>
                          {command.account && <AccountStatus user={command.account} quiet />}
                          {command.hint && (
                            <span className="ml-auto truncate pl-2 text-xs text-muted-foreground">
                              {command.hint}
                            </span>
                          )}
                        </button>
                      )
                    })}
                </div>
              ))
            )}
          </div>

          <div className="flex items-center gap-3 border-t px-3 py-2 text-xs text-muted-foreground">
            <span><kbd className="font-sans">↑</kbd> <kbd className="font-sans">↓</kbd> to move</span>
            <span><kbd className="font-sans">↵</kbd> to open</span>
            <span><kbd className="font-sans">esc</kbd> to close</span>
          </div>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}
