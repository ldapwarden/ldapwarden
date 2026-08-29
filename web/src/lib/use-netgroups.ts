import { useQuery } from '@tanstack/react-query'
import { api } from './api'
import { DEFAULT_MODULES, netgroupsEnabled } from './netgroup'

/**
 * Whether to render netgroup UI on this page.
 *
 * Both conditions in one place because getting either wrong is silent: without
 * the capability the endpoints answer 501 and every panel would sit empty, and
 * without the module the operator has said they do not manage netgroups here.
 * The module used to gate only the navigation entry, so turning it off left the
 * pickers and panels on the sudo and user pages with no way to reach the list
 * they refer to.
 */
export function useNetgroupsEnabled(): boolean {
  const { data: config } = useQuery({
    queryKey: ['admin', 'config'],
    queryFn: ({ signal }) => api.admin.getConfig(signal),
  })

  const modules = config?.app?.modules?.value
  return netgroupsEnabled(
    Array.isArray(modules) ? (modules as string[]) : DEFAULT_MODULES,
    config?.capabilities?.netgroupsSupported ?? false,
  )
}
