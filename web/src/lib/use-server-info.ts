import { useQuery } from '@tanstack/react-query'
import { api, type ServerInfo } from './api'
import { DEFAULT_MODULES, netgroupsEnabled } from './netgroup'

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
  const { data } = useQuery({
    queryKey: ['server-info'],
    queryFn: ({ signal }) => api.serverInfo(signal),
    // A running server does not change its build or its modules under us.
    staleTime: Infinity,
  })
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
  const info = useServerInfo()
  return netgroupsEnabled(
    info?.modules?.length ? info.modules : DEFAULT_MODULES,
    info?.capabilities?.netgroupsSupported ?? false,
  )
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
