import { parseLdapTimestamp } from './utils'

export interface AccountStatusFields {
  accountLocked?: boolean
  pwdAccountLockedTime?: string
  pwdEndTime?: string
}

/**
 * The instant the directory starts refusing this account, from `pwdEndTime`.
 *
 * Only that attribute can answer the question: `pwdAccountLockedTime` is a lock
 * (and OpenLDAP lifts a timestamped one once pwdLockoutDuration elapses), and
 * `shadowExpire` is read by PAM/SSSD on the hosts, never by slapd.
 */
export function accountExpiration(user: AccountStatusFields): Date | null {
  if (!user.pwdEndTime) return null
  return parseLdapTimestamp(user.pwdEndTime)
}
