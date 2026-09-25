import type { AuditLog } from './api'
import { encodeDN } from './utils'

// resourceLink maps an entry back to the page for its resource. Deletions get no
// link: the entry is precisely the record that the target no longer exists.
export function resourceLink(log: AuditLog): { to: string; dn: string } | null {
  if (!log.resourceDn || log.action.includes('delete')) return null
  const route: Record<string, string> = {
    user: '/users/$dn',
    group: '/groups/$dn',
    sudorole: '/sudo-roles/$dn',
    pwdpolicy: '/password-policies/$dn',
    netgroup: '/netgroups/$dn',
  }
  const to = route[log.resourceType]
  return to ? { to, dn: encodeDN(log.resourceDn) } : null
}
