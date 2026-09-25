import { decodeDN } from './utils'

export const APP_NAME = 'LDAP Warden'

const SECTIONS: Record<string, string> = {
  users: 'Users',
  groups: 'Groups',
  'sudo-roles': 'Sudo Roles',
  netgroups: 'Netgroups',
  'password-policies': 'Password Policies',
  'audit-logs': 'Audit Logs',
  admin: 'Administration',
  login: 'Sign in',
  'reset-password': 'Reset password',
}

const SUBPAGES: Record<string, string> = {
  new: 'New',
  import: 'Import',
}

/**
 * The browser-tab title for a path: the section, and on a detail page the
 * entry's name, so a row of tabs reads "jdoe · Users" rather than six
 * identical "Users".
 *
 * The name comes from the DN in the path — its first RDN value is the uid or
 * cn — so no detail page has to set the title itself, and a child effect
 * setting it would be overwritten by the root's anyway.
 */
export function pageTitle(pathname: string): string {
  const [seg, sub] = pathname.split('/').filter(Boolean)
  const section = seg ? SECTIONS[seg] : undefined
  if (!section) return APP_NAME
  if (!sub) return `${section} · ${APP_NAME}`
  const name = SUBPAGES[sub] ?? rdnValue(sub)
  return name ? `${name} · ${section} · ${APP_NAME}` : `${section} · ${APP_NAME}`
}

// rdnValue reads "jdoe" out of the encoded "uid=jdoe,ou=People,…", or returns
// undefined for anything that is not an encoded DN.
function rdnValue(encoded: string): string | undefined {
  let dn: string
  try {
    dn = decodeDN(decodeURIComponent(encoded))
  } catch {
    return undefined
  }
  // Up to the first comma that is not escaped; RFC 4514 escapes are a
  // backslash before the character.
  const match = /^[^=,]+=((?:\\.|[^,\\])+)/.exec(dn)
  return match ? match[1].replace(/\\(.)/g, '$1') : undefined
}
