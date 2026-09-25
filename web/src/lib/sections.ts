import { useMemo } from 'react'
import { KeyRound, Network, ScrollText, ShieldCheck, Users, UsersRound } from 'lucide-react'
import { useAuth } from './auth'
import { useEnabledModules, useServerInfo } from './use-server-info'

export interface SectionAction {
  to: string
  label: string
}

/**
 * One top-level area of the app, and what this session may do in it.
 *
 * The nav bar, the mobile menu and the command palette all list these. They
 * used to decide visibility each on their own: the palette offered Sudo and
 * Netgroups whatever the enabled modules, so on a directory without nisNetgroup
 * it linked to a page that answers 501 and fired a failing search on every
 * keystroke; and it showed Administration on settings:read while the route
 * itself redirects anyone who is not an admin. One list, one set of rules.
 */
export interface Section {
  key: 'users' | 'groups' | 'sudo' | 'netgroups' | 'policies' | 'audit'
  to: string
  label: string
  /** For the top bar, where six full labels do not fit beside the account menu. */
  shortLabel?: string
  icon: typeof Users
  /** The create form, when this session may create here. */
  create?: SectionAction
  /** The CSV import, when this section has one and this session may use it. */
  import?: SectionAction
}

/**
 * Whether the account may open the admin page — the same test the route's
 * beforeLoad applies, so no link to it is ever shown to someone it redirects.
 */
export function useIsAdmin(): boolean {
  const { session } = useAuth()
  return session?.roleName === 'admin'
}

/** The sections this session can reach, in nav order. */
export function useSections(): Section[] {
  const { hasPermission } = useAuth()
  const modules = useEnabledModules()
  const netgroupsSupported = useServerInfo()?.capabilities?.netgroupsSupported ?? false

  return useMemo(() => {
    const sections: Section[] = []
    // Each read permission is the one the list endpoint requires, and each
    // create permission the one its POST requires (internal/api/routes.go).
    if (modules.includes('users') && hasPermission('users:read')) {
      sections.push({
        key: 'users',
        to: '/users',
        label: 'Users',
        icon: Users,
        create: hasPermission('users:write') ? { to: '/users/new', label: 'New User' } : undefined,
        import: hasPermission('users:write') ? { to: '/users/import', label: 'Import Users' } : undefined,
      })
    }
    if (modules.includes('groups') && hasPermission('groups:read')) {
      sections.push({
        key: 'groups',
        to: '/groups',
        label: 'Groups',
        icon: UsersRound,
        create: hasPermission('groups:write') ? { to: '/groups/new', label: 'New Group' } : undefined,
        import: hasPermission('groups:write') ? { to: '/groups/import', label: 'Import Groups' } : undefined,
      })
    }
    if (modules.includes('sudo') && hasPermission('users:read')) {
      sections.push({
        key: 'sudo',
        to: '/sudo-roles',
        label: 'Sudo Roles',
        icon: ShieldCheck,
        create: hasPermission('users:write') ? { to: '/sudo-roles/new', label: 'New Sudo Role' } : undefined,
      })
    }
    // Gated on the directory declaring nisNetgroup as well as on the module: the
    // API answers 501 without it, and a link to a dead page is worse than none.
    if (modules.includes('netgroups') && netgroupsSupported && hasPermission('groups:read')) {
      sections.push({
        key: 'netgroups',
        to: '/netgroups',
        label: 'Netgroups',
        icon: Network,
        create: hasPermission('groups:write') ? { to: '/netgroups/new', label: 'New Netgroup' } : undefined,
        import: hasPermission('groups:write') ? { to: '/netgroups/import', label: 'Import Netgroups' } : undefined,
      })
    }
    if (modules.includes('policies') && hasPermission('settings:read')) {
      sections.push({
        key: 'policies',
        to: '/password-policies',
        label: 'Password Policies',
        shortLabel: 'Policies',
        icon: KeyRound,
        create: hasPermission('settings:write')
          ? { to: '/password-policies/new', label: 'New Password Policy' }
          : undefined,
      })
    }
    if (hasPermission('audit:read')) {
      sections.push({ key: 'audit', to: '/audit-logs', label: 'Audit Logs', icon: ScrollText })
    }
    return sections
  }, [modules, netgroupsSupported, hasPermission])
}
