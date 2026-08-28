import { CalendarClock, CheckCircle2, Lock, LockOpen } from 'lucide-react'
import {
  accountExpiration,
  hasLapsedLock,
  isLocked,
  lockoutLiftsAt,
  type AccountStatusFields,
} from '@/lib/account'

interface Status {
  icon: typeof Lock
  className: string
  label: string
  /**
   * Whether this state is worth interrupting a scan for. Drives `quiet`: a
   * column of green ticks down a member picker is noise, while a locked account
   * in that same list is the whole point of showing status there.
   */
  notable: boolean
}

// lockLabel names the lock behind isLocked(). The wording matches the Security
// tab, so an operator reads the same sentence in the list and on the account.
function lockLabel(user: AccountStatusFields): string {
  switch (user.lockout?.state) {
    case 'manual':
      return 'Locked by an administrator'
    case 'active': {
      const liftsAt = lockoutLiftsAt(user)
      return liftsAt
        ? `Locked by the password policy until ${liftsAt.toLocaleString()}`
        : 'Locked by the password policy, which never lifts it on its own'
    }
    case 'unknown':
      return 'Locked — whether the policy has already lifted it is not known'
    default:
      // No resolved lockout: the row was rendered from `accountLocked` alone,
      // which says the attribute is set and nothing more.
      return 'Account locked'
  }
}

function statusOf(user: AccountStatusFields, now: Date): Status {
  const expiresAt = accountExpiration(user)

  // Two independent states now, and both refuse the bind: a lock
  // (pwdAccountLockedTime, set by an administrator or by ppolicy after too many
  // failures) and a lapsed expiry (pwdEndTime).
  //
  // isLocked, not accountLocked: a ppolicy lockout past its pwdLockoutDuration
  // leaves the attribute behind, and the row would keep reporting a lock the
  // directory stopped honouring.
  if (isLocked(user)) {
    // Which kind of lock, not just that there is one. An administrative lock is
    // deliberate and holds until someone lifts it; a ppolicy lockout is the
    // directory reacting to failed sign-ins and usually clears itself. Reviewing
    // a group's membership, those call for opposite actions, and "Account
    // locked" said neither. Both stay red: both refuse the bind right now, and
    // softening the policy case would understate an account that cannot log in.
    return { icon: Lock, className: 'text-destructive', label: lockLabel(user), notable: true }
  }
  if (expiresAt && expiresAt < now) {
    return { icon: Lock, className: 'text-destructive', label: 'Account expired', notable: true }
  }
  // The account works, but the entry still carries the lapsed lock — say so
  // rather than showing a plain "Active" next to a pwdAccountLockedTime value
  // someone may be reading in phpLDAPadmin.
  if (hasLapsedLock(user)) {
    return {
      icon: LockOpen,
      className: 'text-success',
      label: 'Active — lockout expired, flag not yet cleared',
      // The account works: a picker does not need to stop the eye on it.
      notable: false,
    }
  }

  if (expiresAt) {
    const days = Math.ceil((expiresAt.getTime() - now.getTime()) / (1000 * 60 * 60 * 24))
    return {
      icon: CalendarClock,
      className: days <= 7 ? 'text-destructive' : days <= 30 ? 'text-warning' : 'text-success',
      label: `Expires in ${days} day${days === 1 ? '' : 's'}`,
      notable: days <= 30,
    }
  }

  return { icon: CheckCircle2, className: 'text-success', label: 'Active', notable: false }
}

/**
 * Account status for a list row.
 *
 * The state used to be carried by the icon's colour alone, with the wording
 * only in a `title` attribute — invisible to a screen reader and to anyone who
 * cannot separate red from green. The label is now real text (visually hidden,
 * so the column stays compact) and the glyph is decorative.
 */
export function AccountStatus({
  user,
  quiet = false,
}: {
  user: AccountStatusFields
  /** Render nothing unless the account needs attention. For dense lists. */
  quiet?: boolean
}) {
  const now = new Date()
  const { icon: Icon, className, label, notable } = statusOf(user, now)

  if (quiet && !notable) return null

  return (
    <span className="inline-flex items-center" title={label}>
      <Icon className={`h-4 w-4 ${className}`} aria-hidden="true" />
      <span className="sr-only">{label}</span>
    </span>
  )
}
