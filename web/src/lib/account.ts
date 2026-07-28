import { parseLdapTimestamp } from './utils'

// The conventional "no expiration" ceiling for shadowExpire; the backend uses
// the same bound.
const SHADOW_EXPIRE_NEVER = 99999

export interface AccountStatusFields {
  accountLocked?: boolean
  pwdAccountLockedTime?: string
  shadowExpire?: number
}

/**
 * Expiration date of an account, from whichever attribute records it: the
 * ppolicy operational attribute takes precedence over the shadow one.
 */
export function accountExpiration(user: AccountStatusFields): Date | null {
  if (user.pwdAccountLockedTime) {
    return parseLdapTimestamp(user.pwdAccountLockedTime)
  }
  if (user.shadowExpire && user.shadowExpire > 0 && user.shadowExpire < SHADOW_EXPIRE_NEVER) {
    return new Date(user.shadowExpire * 24 * 60 * 60 * 1000)
  }
  return null
}
