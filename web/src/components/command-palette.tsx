import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import * as DialogPrimitive from '@radix-ui/react-dialog'
import {
  KeyRound,
  Plus,
  ScrollText,
  Search,
  Settings,
  ShieldCheck,
  Upload,
  User,
  Users,
  UsersRound,
} from 'lucide-react'
import { api } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { useDebounced } from '@/lib/use-debounced'
import { encodeDN } from '@/lib/utils'
import { cn } from '@/lib/utils'

interface Command {
  id: string
  label: string
  hint?: string
  group: string
  icon: typeof Users
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
  const { hasPermission } = useAuth()
  const [query, setQuery] = useState('')
  const debouncedQuery = useDebounced(query)
  const [activeIndex, setActiveIndex] = useState(0)
  const listRef = useRef<HTMLDivElement>(null)

  const canWrite = hasPermission('users:write')
  const canReadAudit = hasPermission('audit:read')
  const canReadSettings = hasPermission('settings:read')

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
    enabled: open && searching,
  })

  const { data: groups } = useQuery({
    queryKey: ['groups', { search: term, limit: perGroupLimit, palette: true }],
    queryFn: ({ signal }) => api.groups.list({ search: term, limit: perGroupLimit }, signal),
    enabled: open && searching,
  })

  const { data: sudoRoles } = useQuery({
    queryKey: ['sudo-roles', { search: term, palette: true }],
    queryFn: ({ signal }) => api.sudoRoles.list(term, signal),
    enabled: open && searching,
  })

  const { data: policies } = useQuery({
    queryKey: ['password-policies', { search: term, palette: true }],
    queryFn: ({ signal }) => api.passwordPolicies.list(term, signal),
    enabled: open && searching && canReadSettings,
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
      for (const role of (sudoRoles?.data ?? []).slice(0, perGroupLimit)) {
        items.push({
          id: `sudo:${role.dn}`,
          label: role.cn,
          hint: role.description,
          group: 'Sudo roles',
          icon: ShieldCheck,
          run: go('/sudo-roles/$dn', { dn: encodeDN(role.dn) }),
        })
      }
      for (const policy of (policies?.data ?? []).slice(0, perGroupLimit)) {
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

    // Navigation and create actions. Shown on their own when the palette opens,
    // and filtered by the term once the user starts typing, so "new" reaches the
    // create forms without leaving the keyboard.
    const actions: Command[] = [
      { id: 'go:users', label: 'Users', group: 'Go to', icon: Users, run: go('/users') },
      { id: 'go:groups', label: 'Groups', group: 'Go to', icon: UsersRound, run: go('/groups') },
      { id: 'go:sudo', label: 'Sudo roles', group: 'Go to', icon: ShieldCheck, run: go('/sudo-roles') },
    ]
    if (canReadSettings) {
      actions.push({ id: 'go:policies', label: 'Password policies', group: 'Go to', icon: KeyRound, run: go('/password-policies') })
      actions.push({ id: 'go:admin', label: 'Administration', group: 'Go to', icon: Settings, run: go('/admin') })
    }
    if (canReadAudit) {
      actions.push({ id: 'go:audit', label: 'Audit log', group: 'Go to', icon: ScrollText, run: go('/audit-logs') })
    }
    if (canWrite) {
      actions.push(
        { id: 'new:user', label: 'New user', group: 'Create', icon: Plus, run: go('/users/new') },
        { id: 'new:group', label: 'New group', group: 'Create', icon: Plus, run: go('/groups/new') },
        { id: 'new:sudo', label: 'New sudo role', group: 'Create', icon: Plus, run: go('/sudo-roles/new') },
        { id: 'import:users', label: 'Import users from CSV', group: 'Create', icon: Upload, run: go('/users/import') },
        { id: 'import:groups', label: 'Import groups from CSV', group: 'Create', icon: Upload, run: go('/groups/import') },
      )
    }

    const needle = term.toLowerCase()
    items.push(...actions.filter((action) => !searching || action.label.toLowerCase().includes(needle)))

    return items
  }, [
    searching, term, users, groups, sudoRoles, policies,
    canWrite, canReadAudit, canReadSettings, navigate, onOpenChange,
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
