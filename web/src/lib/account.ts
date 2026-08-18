import type { Lockout } from './api'
import { parseLdapTimestamp } from './utils'

export interface AccountStatusFields {
  accountLocked?: boolean
  pwdAccountLockedTime?: string
  pwdEndTime?: string
  /**
   * The backend's reading of `pwdAccountLockedTime`, resolved against the policy
   * that applies to the entry. Optional so a view that has not asked for it
   * still renders, falling back to `accountLocked`.
   */
  lockout?: Lockout
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

/**
 * Whether a lock is refusing this account's binds right now.
 *
 * Not the same question as `accountLocked`, which only reports that
 * `pwdAccountLockedTime` is set. ppolicy leaves a lapsed lockout in the entry
 * until the next bind on it, so the attribute outlives the lock — by hours or
 * by weeks — and reading it as "locked" is what used to make this UI claim a
 * lockout that had already expired.
 *
 * An unresolved lock ('unknown') counts as locked: it might be.
 */
export function isLocked(user: AccountStatusFields): boolean {
  if (user.lockout) {
    return user.lockout.state !== 'none' && user.lockout.state !== 'lifted'
  }
  return user.accountLocked ?? false
}

/**
 * Whether the entry carries a lock the directory has stopped honouring and not
 * yet removed. Worth saying out loud: the account works, but `Unlock` is still
 * offered, because only a write (or the next bind) clears the attribute.
 */
export function hasLapsedLock(user: AccountStatusFields): boolean {
  return user.lockout?.state === 'lifted'
}

/**
 * When the lockout duration runs out — in the past when the lock has lapsed.
 * Null when there is no lock, or when the applicable policy is unknown, or when
 * the duration is 0 and the lock therefore never lifts.
 */
export function lockoutLiftsAt(user: AccountStatusFields): Date | null {
  const liftsAt = user.lockout?.liftsAt
  if (!liftsAt) return null
  const parsed = new Date(liftsAt)
  return Number.isNaN(parsed.getTime()) ? null : parsed
}
