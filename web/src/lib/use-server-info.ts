import { queryOptions, useQuery, type QueryClient } from '@tanstack/react-query'
import { redirect } from '@tanstack/react-router'
import { api, type ServerInfo } from './api'
import { DEFAULT_MODULES, netgroupsEnabled } from './netgroup'

// The one query behind every read of server-info, from a component or a route.
export const serverInfoQuery = queryOptions({
  queryKey: ['server-info'],
  queryFn: ({ signal }) => api.serverInfo(signal),
  // A running server does not change its build or its modules under us.
  staleTime: Infinity,
})

export type Module = 'users' | 'groups' | 'sudo' | 'policies' | 'netgroups'

/**
 * Whether a module's pages exist in this deployment: the operator enabled it,
 * and — for netgroups — the directory can hold them. The single rule behind
 * both the sections listed (useSections) and the routes that refuse to render
 * (requireModule), so a module cannot be hidden from one and reachable in the
 * other.
 */
export function moduleAvailable(module: Module, info: ServerInfo | undefined): boolean {
  const modules = info?.modules?.length ? info.modules : DEFAULT_MODULES
  if (module === 'netgroups') {
    return netgroupsEnabled(modules, info?.capabilities?.netgroupsSupported ?? false)
  }
  return modules.includes(module)
}

/**
 * Route guard for a module's pages. A disabled module's pages used to stay
 * usable by URL, with only the nav hiding them; they now redirect, as a page
 * the session lacks the permission for does. The API still serves them — a
 * module is the operator's choice of what to manage here, not a security
 * boundary.
 *
 * When server-info cannot be read the page is let through: it will report its
 * own failure, which says more than a redirect would.
 */
export async function requireModule(queryClient: QueryClient, module: Module): Promise<void> {
  let info: ServerInfo
  try {
    info = await queryClient.ensureQueryData(serverInfoQuery)
  } catch {
    return
  }
  if (!moduleAvailable(module, info)) {
    throw redirect({ to: '/' })
  }
}

/**
 * Which build is running and what this session may render.
 *
 * One query, shared by every caller through the cache. It replaces the reads of
 * /admin/config that used to decide what the UI shows: that endpoint takes
 * settings:read, which the readonly role does not have, so a read-only user was
 * served a UI missing features the API would have let them use — silently, since
 * a 403 there just left the answer undefined and everything defaulted to off.
 */
export function useServerInfo(): ServerInfo | undefined {
  const { data } = useQuery(serverInfoQuery)
  return data
}

/** The enabled modules, falling back to the defaults until the answer arrives. */
export function useEnabledModules(): string[] {
  const info = useServerInfo()
  return info?.modules?.length ? info.modules : DEFAULT_MODULES
}

/**
 * Whether the netgroup UI should appear at all.
 *
 * Two independent conditions: the operator enabled the module, and the directory
 * can actually hold netgroups. Without the second the API answers 501, so the UI
 * would lead nowhere; without the first the operator has said they do not manage
 * netgroups here.
 */
export function useNetgroupsEnabled(): boolean {
  return moduleAvailable('netgroups', useServerInfo())
}

/**
 * The object classes a page may offer tabs for, lower-cased for comparison.
 *
 * Same story as the modules: these drove which tabs the user and group pages
 * show, they came from /admin/config, and a read-only session silently got the
 * hardcoded fallback instead of the deployment's own configuration.
 */
export function useEnabledObjects(kind: 'users' | 'groups', fallback: string[]): Set<string> {
  const info = useServerInfo()
  const configured = kind === 'users' ? info?.usersObjects : info?.groupsObjects
  const values = configured?.length ? configured : fallback
  return new Set(values.map((value) => value.toLowerCase()))
}
